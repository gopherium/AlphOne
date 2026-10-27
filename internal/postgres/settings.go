// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/encryption"
	"github.com/gopherium/alphone/internal/postgres/db"
	"github.com/gopherium/alphone/sdk"
)

var (
	_ sdk.SettingStore = (*SettingStore)(nil)
	_ sdk.SettingStore = (*prefixedSettings)(nil)
)

// pluginKeyPrefix leads every key a plugin's settings store keeps, before the plugin id.
const pluginKeyPrefix = "plugin."

// secretPurpose leads the associated data every stored secret is encrypted with.
const secretPurpose = "setting"

// SettingStore keeps the settings of every workspace, its secrets encrypted with the keys it holds.
type SettingStore struct {
	prefixedSettings
}

// prefixedSettings keeps the settings whose full keys start with prefix, each key taken and answered without it.
type prefixedSettings struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	keys    *encryption.Keys
	prefix  string
}

// NewSettingStore returns a SettingStore backed by pool that encrypts its secrets with keys.
func NewSettingStore(pool *pgxpool.Pool, keys *encryption.Keys) *SettingStore {
	return &SettingStore{prefixedSettings{pool: pool, queries: db.New(pool), keys: keys}}
}

// ForPlugin answers the settings store of the plugin called id, every key kept under its own prefix.
func (s *SettingStore) ForPlugin(id string) sdk.SettingStore {
	plugin := s.prefixedSettings
	plugin.prefix = pluginKeyPrefix + id + "."
	return &plugin
}

// Lookup answers the plain value under key and whether it is set, ErrSecretSetting when it holds a secret.
func (s *prefixedSettings) Lookup(ctx context.Context, key string) (string, bool, error) {
	held, found, err := s.held(ctx, key)
	if err != nil || !found {
		return "", false, err
	}
	if !held.Value.Valid {
		return "", false, fmt.Errorf("postgres: read setting %s: %w", key, sdk.ErrSecretSetting)
	}
	return held.Value.String, true, nil
}

// Save stores every plain value or none of them.
func (s *prefixedSettings) Save(ctx context.Context, values map[string]string) error {
	tenant := sdk.TenantOrDefault(ctx)
	rows := make([]db.SaveSettingParams, 0, len(values))
	for _, key := range slices.Sorted(maps.Keys(values)) {
		rows = append(rows, db.SaveSettingParams{
			TenantID: tenant, Key: s.prefix + key, Value: pgtype.Text{String: values[key], Valid: true},
		})
	}
	return s.write(ctx, rows)
}

// LookupSecret answers the secret under key and whether it is set, ErrPlainSetting when it holds a plain value.
func (s *prefixedSettings) LookupSecret(ctx context.Context, key string) (string, bool, error) {
	held, found, err := s.held(ctx, key)
	if err != nil || !found {
		return "", false, err
	}
	if held.Value.Valid {
		return "", false, fmt.Errorf("postgres: read secret setting %s: %w", key, sdk.ErrPlainSetting)
	}
	plaintext, err := s.keys.Decrypt(held.Encrypted, secretBinding(sdk.TenantOrDefault(ctx), s.prefix+key)...)
	if err != nil {
		return "", false, fmt.Errorf("postgres: read secret setting %s: %w", key, encryptionError(err))
	}
	return string(plaintext), true, nil
}

// SaveSecrets encrypts and stores every secret or none of them.
func (s *prefixedSettings) SaveSecrets(ctx context.Context, secrets map[string]string) error {
	tenant := sdk.TenantOrDefault(ctx)
	rows := make([]db.SaveSettingParams, 0, len(secrets))
	for _, key := range slices.Sorted(maps.Keys(secrets)) {
		encrypted, err := s.keys.Encrypt([]byte(secrets[key]), secretBinding(tenant, s.prefix+key)...)
		if err != nil {
			return fmt.Errorf("postgres: save secret setting %s: %w", key, encryptionError(err))
		}
		rows = append(rows, db.SaveSettingParams{TenantID: tenant, Key: s.prefix + key, Encrypted: encrypted})
	}
	return s.write(ctx, rows)
}

// HasSecret answers whether key holds a secret that is not empty, without handing it out.
func (s *prefixedSettings) HasSecret(ctx context.Context, key string) (bool, error) {
	secret, _, err := s.LookupSecret(ctx, key)
	return secret != "", err
}

// LookupPrefix answers the plain values whose keys start with prefix, leaving secrets out.
func (s *prefixedSettings) LookupPrefix(ctx context.Context, prefix string) (map[string]string, error) {
	rows, err := s.queries.SettingsUnder(ctx, db.SettingsUnderParams{
		TenantID: sdk.TenantOrDefault(ctx), Prefix: s.prefix + prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: read settings under %s: %w", prefix, err)
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[strings.TrimPrefix(row.Key, s.prefix)] = row.Value.String
	}
	return values, nil
}

// held answers the row stored under key in the calling workspace and whether one is stored.
func (s *prefixedSettings) held(ctx context.Context, key string) (db.SettingRow, bool, error) {
	row, err := s.queries.Setting(ctx, db.SettingParams{TenantID: sdk.TenantOrDefault(ctx), Key: s.prefix + key})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SettingRow{}, false, nil
	}
	if err != nil {
		return db.SettingRow{}, false, fmt.Errorf("postgres: read setting %s: %w", key, err)
	}
	return row, true, nil
}

// write upserts every row in one transaction, in the order given, storing none when one fails.
func (s *prefixedSettings) write(ctx context.Context, rows []db.SaveSettingParams) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: save settings: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := s.queries.WithTx(tx)
	for _, row := range rows {
		if err := queries.SaveSetting(ctx, row); err != nil {
			return fmt.Errorf("postgres: save setting %s: %w", strings.TrimPrefix(row.Key, s.prefix), err)
		}
	}
	return tx.Commit(ctx)
}

// secretBinding answers the associated data a secret stored under the full key in the workspace is encrypted with.
func secretBinding(tenant uuid.UUID, key string) [][]byte {
	return [][]byte{[]byte(secretPurpose), tenant[:], []byte(key)}
}

// encryptionError answers an error of the encryption keys as the sdk error a plugin reads.
func encryptionError(err error) error {
	if errors.Is(err, encryption.ErrNoKey) {
		return sdk.ErrNoEncryptionKey
	}
	return fmt.Errorf("%w: %w", sdk.ErrCannotDecrypt, err)
}

// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/postgres/db"
	"github.com/gopherium/alphone/sdk"
)

// UserSettingStore reads and writes the per-user settings.
type UserSettingStore struct {
	queries *db.Queries
}

// NewUserSettingStore returns a UserSettingStore backed by pool.
func NewUserSettingStore(pool *pgxpool.Pool) *UserSettingStore {
	return &UserSettingStore{queries: db.New(pool)}
}

// Lookup answers the value stored under key for the user and whether one is stored.
func (s *UserSettingStore) Lookup(ctx context.Context, userID uuid.UUID, key string) (string, bool, error) {
	values, err := s.queries.UserSetting(ctx, db.UserSettingParams{
		UserID: userID, Key: key, TenantID: sdk.TenantOrDefault(ctx),
	})
	if err != nil {
		return "", false, fmt.Errorf("read setting %s: %w", key, err)
	}
	if len(values) == 0 {
		return "", false, nil
	}
	return values[0], true, nil
}

// Save stores value under key for the user, replacing any earlier value.
func (s *UserSettingStore) Save(ctx context.Context, userID uuid.UUID, key, value string) error {
	err := s.queries.SetUserSetting(ctx, db.SetUserSettingParams{
		UserID: userID, Key: key, Value: value, TenantID: sdk.TenantOrDefault(ctx),
	})
	if err != nil {
		return fmt.Errorf("store setting %s: %w", key, err)
	}
	return nil
}

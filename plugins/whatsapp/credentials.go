// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// credentials carries the Graph API identity one tenant sends with.
type credentials struct {
	phoneNumberID string
	accessToken   string
}

// Credential errors.
var (
	errNoCredentialsKey = errors.New("whatsapp: ALPHONE_WHATSAPP_CREDENTIALS_KEY is not set")
	errNoCredentials    = errors.New("whatsapp: no credentials for the tenant")
	errSealTooShort     = errors.New("whatsapp: sealed token shorter than its nonce")
)

// credentialsKey parses a hex encoded 32 byte sealing key, answering nil for an empty value.
func credentialsKey(raw string) ([]byte, error) {
	if raw == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: parse ALPHONE_WHATSAPP_CREDENTIALS_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("whatsapp: ALPHONE_WHATSAPP_CREDENTIALS_KEY holds %d bytes, want 32", len(key))
	}
	return key, nil
}

// defaultRandRead is the entropy source a nonce is drawn from.
var defaultRandRead = rand.Read

// randRead draws the entropy a nonce is drawn from.
var randRead = defaultRandRead

// mustGCM returns the sealer over block, panicking when the block cannot carry one.
func mustGCM(block cipher.Block) cipher.AEAD {
	sealer, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return sealer
}

// tokenCipher builds the AES-GCM sealer the key unlocks.
func tokenCipher(key []byte) (cipher.AEAD, error) {
	if len(key) == 0 {
		return nil, errNoCredentialsKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: build cipher: %w", err)
	}
	return mustGCM(block), nil
}

// sealToken encrypts token under key, prepending the random nonce.
func sealToken(key []byte, token string) ([]byte, error) {
	sealer, err := tokenCipher(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, sealer.NonceSize())
	if _, err := randRead(nonce); err != nil {
		return nil, fmt.Errorf("whatsapp: draw nonce: %w", err)
	}
	return sealer.Seal(nonce, nonce, []byte(token), nil), nil
}

// openToken decrypts a sealed token under key.
func openToken(key []byte, sealed []byte) (string, error) {
	sealer, err := tokenCipher(key)
	if err != nil {
		return "", err
	}
	if len(sealed) < sealer.NonceSize() {
		return "", errSealTooShort
	}
	token, err := sealer.Open(nil, sealed[:sealer.NonceSize()], sealed[sealer.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("whatsapp: open token: %w", err)
	}
	return string(token), nil
}

// upsertCredentials stores the calling tenant's number and sealed token.
func (s *store) upsertCredentials(ctx context.Context, phoneNumberID string, sealed []byte) error {
	_, err := s.pool.Exec(ctx,
		"INSERT INTO plugin_whatsapp.credentials (tenant_id, phone_number_id, access_token)"+
			" VALUES ($1, $2, $3)"+
			" ON CONFLICT (tenant_id) DO UPDATE"+
			" SET phone_number_id = excluded.phone_number_id, access_token = excluded.access_token,"+
			" updated_at = now()",
		sdk.TenantOrDefault(ctx), phoneNumberID, sealed)
	if err != nil {
		return fmt.Errorf("whatsapp: store credentials: %w", err)
	}
	return nil
}

// sealedCredentials returns the calling tenant's stored number and sealed token.
func (s *store) sealedCredentials(ctx context.Context) (string, []byte, error) {
	var phoneNumberID string
	var sealed []byte
	err := s.pool.QueryRow(ctx,
		"SELECT phone_number_id, access_token FROM plugin_whatsapp.credentials WHERE tenant_id = $1",
		sdk.TenantOrDefault(ctx)).Scan(&phoneNumberID, &sealed)
	if err != nil {
		return "", nil, err
	}
	return phoneNumberID, sealed, nil
}

// tenantForPhoneNumber returns the tenant owning the given number.
func (s *store) tenantForPhoneNumber(ctx context.Context, phoneNumberID string) (uuid.UUID, error) {
	var tenantID uuid.UUID
	err := s.pool.QueryRow(ctx,
		"SELECT tenant_id FROM plugin_whatsapp.credentials WHERE phone_number_id = $1",
		phoneNumberID).Scan(&tenantID)
	if err != nil {
		return uuid.Nil, err
	}
	return tenantID, nil
}

// SetCredentials stores the calling tenant's WhatsApp number and access token, sealed at rest.
func (p *Plugin) SetCredentials(ctx context.Context, phoneNumberID, accessToken string) error {
	if phoneNumberID == "" || accessToken == "" {
		return errors.New("whatsapp: credentials must carry a number and a token")
	}
	sealed, err := sealToken(p.key, accessToken)
	if err != nil {
		return err
	}
	return p.store.upsertCredentials(ctx, phoneNumberID, sealed)
}

// routeByNumber answers the context serving the number's tenant and whether any tenant owns it.
func (p *Plugin) routeByNumber(ctx context.Context, phoneNumberID string) (context.Context, bool, error) {
	if phoneNumberID == "" {
		return ctx, true, nil
	}
	tenantID, err := p.store.tenantForPhoneNumber(ctx, phoneNumberID)
	if err == nil {
		return sdk.WithTenant(ctx, tenantID), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ctx, false, fmt.Errorf("whatsapp: route number: %w", err)
	}
	if phoneNumberID == p.envCredentials.phoneNumberID {
		return ctx, true, nil
	}
	return ctx, false, nil
}

// credentialsFor answers the calling tenant's credentials, the env seed for the default tenant.
func (p *Plugin) credentialsFor(ctx context.Context) (credentials, error) {
	phoneNumberID, sealed, err := p.store.sealedCredentials(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		if sdk.TenantOrDefault(ctx) == sdk.DefaultTenantID {
			return p.envCredentials, nil
		}
		return credentials{}, errNoCredentials
	}
	if err != nil {
		return credentials{}, fmt.Errorf("whatsapp: read credentials: %w", err)
	}
	token, err := openToken(p.key, sealed)
	if err != nil {
		return credentials{}, err
	}
	return credentials{phoneNumberID: phoneNumberID, accessToken: token}, nil
}

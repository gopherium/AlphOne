// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// credentials carries the Graph API identity one number sends with.
type credentials struct {
	phoneNumberID string
	accessToken   string
}

// complete reports whether the credentials carry both halves of the identity.
func (c credentials) complete() bool {
	return c.phoneNumberID != "" && c.accessToken != ""
}

// Credential errors.
var (
	errNoCredentials = errors.New("whatsapp: no credentials for the tenant")
	errNumberRemoved = errors.New("whatsapp: that number is no longer connected")
	errNumberTaken   = errors.New("whatsapp: another workspace holds that number")
)

// accessTokenPurpose names what an access token is to the encrypter.
const accessTokenPurpose = "access_token"

// appSecretPurpose names what an app secret is to the encrypter.
const appSecretPurpose = "app_secret"

// storedNumber is one connected number as the store holds it, its secrets encrypted or digested.
type storedNumber struct {
	phoneNumberID     string
	label             string
	accessToken       []byte
	appSecret         []byte
	verifyTokenDigest []byte
}

// upsertCredentials adds the number to the calling tenant or replaces it, answering false when another tenant holds it.
func (s *store) upsertCredentials(ctx context.Context, number storedNumber) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		"INSERT INTO plugin_whatsapp.credentials"+
			" (tenant_id, phone_number_id, access_token, label, app_secret, verify_token_digest)"+
			" VALUES ($1, $2, $3, $4, $5, $6)"+
			" ON CONFLICT (phone_number_id) DO UPDATE SET"+
			" access_token = excluded.access_token,"+
			" label = COALESCE(NULLIF(excluded.label, ''), credentials.label),"+
			" app_secret = COALESCE(excluded.app_secret, credentials.app_secret),"+
			" verify_token_digest = COALESCE(excluded.verify_token_digest, credentials.verify_token_digest),"+
			" updated_at = now()"+
			" WHERE credentials.tenant_id = excluded.tenant_id",
		sdk.TenantOrDefault(ctx), number.phoneNumberID, number.accessToken, number.label,
		number.appSecret, number.verifyTokenDigest)
	if err != nil {
		return false, fmt.Errorf("whatsapp: store credentials: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// encryptedCredentials returns the calling tenant's number and encrypted token, its oldest for an empty number.
func (s *store) encryptedCredentials(ctx context.Context, number string) (string, []byte, error) {
	var phoneNumberID string
	var encrypted []byte
	err := s.pool.QueryRow(ctx,
		"SELECT phone_number_id, access_token FROM plugin_whatsapp.credentials"+
			" WHERE tenant_id = $1 AND ($2 = '' OR phone_number_id = $2)"+
			" ORDER BY created_at, phone_number_id LIMIT 1",
		sdk.TenantOrDefault(ctx), number).Scan(&phoneNumberID, &encrypted)
	if err != nil {
		return "", nil, err
	}
	return phoneNumberID, encrypted, nil
}

// listNumbers returns the calling tenant's numbers, oldest first.
func (s *store) listNumbers(ctx context.Context) ([]sdk.ChannelIdentity, error) {
	rows, _ := s.pool.Query(ctx,
		"SELECT phone_number_id, label FROM plugin_whatsapp.credentials"+
			" WHERE tenant_id = $1 ORDER BY created_at, phone_number_id",
		sdk.TenantOrDefault(ctx))
	identities, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (sdk.ChannelIdentity, error) {
		var identity sdk.ChannelIdentity
		err := row.Scan(&identity.Identifier, &identity.Label)
		return identity, err
	})
	if err != nil {
		return nil, fmt.Errorf("whatsapp: list numbers: %w", err)
	}
	return identities, nil
}

// deleteCredentials removes one of the calling tenant's numbers.
func (s *store) deleteCredentials(ctx context.Context, number string) error {
	_, err := s.pool.Exec(ctx,
		"DELETE FROM plugin_whatsapp.credentials WHERE tenant_id = $1 AND phone_number_id = $2",
		sdk.TenantOrDefault(ctx), number)
	if err != nil {
		return fmt.Errorf("whatsapp: remove credentials: %w", err)
	}
	return nil
}

// holdsVerifyDigest reports whether any stored number answers to the digest of a verify token.
func (s *store) holdsVerifyDigest(ctx context.Context, digest []byte) (bool, error) {
	var held bool
	err := s.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM plugin_whatsapp.credentials WHERE verify_token_digest = $1)",
		digest).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("whatsapp: read verify tokens: %w", err)
	}
	return held, nil
}

// UseTenantGate receives the host's tenant gate.
func (p *Plugin) UseTenantGate(gate sdk.TenantGate) {
	p.gate = gate
}

var _ sdk.EncrypterConsumer = (*Plugin)(nil)

// UseEncrypter receives the host's encrypter for the plugin.
func (p *Plugin) UseEncrypter(encrypter sdk.Encrypter) {
	p.encrypter = encrypter
}

// recordsTraffic reports whether the tenant still records what a channel delivers.
func (p *Plugin) recordsTraffic(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	if p.gate == nil {
		return true, nil
	}
	return p.gate.AcceptsMachineTraffic(ctx, tenantID)
}

var _ sdk.CredentialProvider = (*Plugin)(nil)

// Channel names the channel the plugin's credentials send on.
func (p *Plugin) Channel() sdk.Channel {
	return "whatsapp"
}

// SetChannelCredentials adds a number to the calling tenant or replaces it, encrypted at rest.
func (p *Plugin) SetChannelCredentials(ctx context.Context, stored sdk.ChannelCredentials) error {
	if stored.Identifier == "" || stored.Secret == "" {
		return errors.New("whatsapp: credentials must carry a number and a token")
	}
	if err := p.guardEnvNumber(ctx, stored.Identifier); err != nil {
		return err
	}
	number, err := p.sealNumber(ctx, stored)
	if err != nil {
		return err
	}
	held, err := p.store.upsertCredentials(ctx, number)
	if err != nil {
		return err
	}
	if !held {
		return numberTaken()
	}
	return nil
}

// guardEnvNumber refuses the env number to any tenant but the default one, unless that tenant already holds its row.
func (p *Plugin) guardEnvNumber(ctx context.Context, number string) error {
	if number != p.envCredentials.phoneNumberID || sdk.TenantOrDefault(ctx) == sdk.DefaultTenantID {
		return nil
	}
	_, _, err := p.store.encryptedCredentials(ctx, number)
	if errors.Is(err, pgx.ErrNoRows) {
		return numberTaken()
	}
	if err != nil {
		return fmt.Errorf("whatsapp: read credentials: %w", err)
	}
	return nil
}

// numberTaken answers that another workspace holds a number, naming no workspace.
func numberTaken() error {
	return sdk.GraphError{Code: "CONFLICT", Reason: "channel_number_taken", Err: errNumberTaken}
}

// sealNumber answers what the store keeps of the given credentials.
func (p *Plugin) sealNumber(ctx context.Context, stored sdk.ChannelCredentials) (storedNumber, error) {
	accessToken, err := p.encrypter.Encrypt(ctx, accessTokenPurpose, []byte(stored.Secret))
	if err != nil {
		return storedNumber{}, fmt.Errorf("whatsapp: encrypt the access token: %w", err)
	}
	number := storedNumber{phoneNumberID: stored.Identifier, label: stored.Label, accessToken: accessToken}
	if stored.AppSecret != "" {
		number.appSecret, err = p.encrypter.Encrypt(ctx, appSecretPurpose, []byte(stored.AppSecret))
		if err != nil {
			return storedNumber{}, fmt.Errorf("whatsapp: encrypt the app secret: %w", err)
		}
	}
	if stored.VerifyToken != "" {
		digest := sha256.Sum256([]byte(stored.VerifyToken))
		number.verifyTokenDigest = digest[:]
	}
	return number, nil
}

// ChannelIdentities lists the calling tenant's numbers, oldest first, or else the env number it serves.
func (p *Plugin) ChannelIdentities(ctx context.Context) ([]sdk.ChannelIdentity, error) {
	identities, err := p.store.listNumbers(ctx)
	if err != nil {
		return nil, err
	}
	if len(identities) == 0 && p.servesEnvNumber(ctx) {
		return []sdk.ChannelIdentity{{Identifier: p.envCredentials.phoneNumberID}}, nil
	}
	return identities, nil
}

// RemoveChannelCredentials removes one of the calling tenant's numbers.
func (p *Plugin) RemoveChannelCredentials(ctx context.Context, identifier string) error {
	return p.store.deleteCredentials(ctx, identifier)
}

// servesEnvNumber reports whether the calling tenant answers the number the environment file names.
func (p *Plugin) servesEnvNumber(ctx context.Context) bool {
	return sdk.TenantOrDefault(ctx) == sdk.DefaultTenantID && p.envCredentials.complete()
}

// envNumber answers the number the environment file names for the calling tenant, empty for none.
func (p *Plugin) envNumber(ctx context.Context) string {
	if sdk.TenantOrDefault(ctx) != sdk.DefaultTenantID {
		return ""
	}
	return p.envCredentials.phoneNumberID
}

// credentialsFor answers the calling tenant's credentials for a number, empty meaning its env number, else its oldest.
func (p *Plugin) credentialsFor(ctx context.Context, number string) (credentials, error) {
	if number == "" {
		number = p.envNumber(ctx)
	}
	phoneNumberID, encrypted, err := p.store.encryptedCredentials(ctx, number)
	if errors.Is(err, pgx.ErrNoRows) {
		return p.credentialsWithoutARow(ctx, number)
	}
	if err != nil {
		return credentials{}, fmt.Errorf("whatsapp: read credentials: %w", err)
	}
	token, err := p.encrypter.Decrypt(ctx, accessTokenPurpose, encrypted)
	if err != nil {
		return credentials{}, fmt.Errorf("whatsapp: decrypt the access token: %w", err)
	}
	return credentials{phoneNumberID: phoneNumberID, accessToken: string(token)}, nil
}

// credentialsWithoutARow answers a number the calling tenant holds no row for, its env number included.
func (p *Plugin) credentialsWithoutARow(ctx context.Context, number string) (credentials, error) {
	switch {
	case number != p.envNumber(ctx):
		return credentials{}, errNumberRemoved
	case number == "" || !p.envCredentials.complete():
		return credentials{}, errNoCredentials
	default:
		return p.envCredentials, nil
	}
}

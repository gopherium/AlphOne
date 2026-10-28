// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
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

// complete reports whether the credentials carry both halves of the identity.
func (c credentials) complete() bool {
	return c.phoneNumberID != "" && c.accessToken != ""
}

// errNoCredentials reports a tenant that holds no credentials.
var errNoCredentials = errors.New("whatsapp: no credentials for the tenant")

// accessTokenPurpose names what an access token is to the encrypter.
const accessTokenPurpose = "access_token"

// upsertCredentials stores the calling tenant's number and encrypted token.
func (s *store) upsertCredentials(ctx context.Context, phoneNumberID string, encrypted []byte) error {
	_, err := s.pool.Exec(ctx,
		"INSERT INTO plugin_whatsapp.credentials (tenant_id, phone_number_id, access_token)"+
			" VALUES ($1, $2, $3)"+
			" ON CONFLICT (tenant_id) DO UPDATE"+
			" SET phone_number_id = excluded.phone_number_id, access_token = excluded.access_token,"+
			" updated_at = now()",
		sdk.TenantOrDefault(ctx), phoneNumberID, encrypted)
	if err != nil {
		return fmt.Errorf("whatsapp: store credentials: %w", err)
	}
	return nil
}

// encryptedCredentials returns the calling tenant's stored number and encrypted token.
func (s *store) encryptedCredentials(ctx context.Context) (string, []byte, error) {
	var phoneNumberID string
	var encrypted []byte
	err := s.pool.QueryRow(ctx,
		"SELECT phone_number_id, access_token FROM plugin_whatsapp.credentials WHERE tenant_id = $1",
		sdk.TenantOrDefault(ctx)).Scan(&phoneNumberID, &encrypted)
	if err != nil {
		return "", nil, err
	}
	return phoneNumberID, encrypted, nil
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

// SetCredentials stores the calling tenant's WhatsApp number and access token, encrypted at rest.
func (p *Plugin) SetCredentials(ctx context.Context, phoneNumberID, accessToken string) error {
	if phoneNumberID == "" || accessToken == "" {
		return errors.New("whatsapp: credentials must carry a number and a token")
	}
	encrypted, err := p.encrypter.Encrypt(ctx, accessTokenPurpose, []byte(accessToken))
	if err != nil {
		return fmt.Errorf("whatsapp: encrypt the access token: %w", err)
	}
	return p.store.upsertCredentials(ctx, phoneNumberID, encrypted)
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

// routeByNumber answers the context serving the number's tenant and whether any tenant owns it.
func (p *Plugin) routeByNumber(ctx context.Context, phoneNumberID string) (context.Context, bool, error) {
	routed, owned, err := p.numberTenant(ctx, phoneNumberID)
	if err != nil || !owned {
		return ctx, false, err
	}
	recording, err := p.recordsTraffic(routed, sdk.TenantOrDefault(routed))
	if err != nil || !recording {
		return ctx, false, err
	}
	return routed, true, nil
}

// numberTenant answers the context serving the number's tenant and whether any tenant owns it.
func (p *Plugin) numberTenant(ctx context.Context, phoneNumberID string) (context.Context, bool, error) {
	if phoneNumberID == "" {
		if p.envCredentials.phoneNumberID == "" {
			return ctx, false, nil
		}
		return sdk.WithTenant(ctx, sdk.DefaultTenantID), true, nil
	}
	tenantID, err := p.store.tenantForPhoneNumber(ctx, phoneNumberID)
	if err == nil {
		return sdk.WithTenant(ctx, tenantID), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ctx, false, fmt.Errorf("whatsapp: route number: %w", err)
	}
	if phoneNumberID == p.envCredentials.phoneNumberID {
		return sdk.WithTenant(ctx, sdk.DefaultTenantID), true, nil
	}
	return ctx, false, nil
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

// SetChannelCredentials stores the calling tenant's number and token, encrypted at rest.
func (p *Plugin) SetChannelCredentials(ctx context.Context, identifier, secret string) error {
	return p.SetCredentials(ctx, identifier, secret)
}

// ChannelIdentifier answers the calling tenant's configured number and whether one is set.
func (p *Plugin) ChannelIdentifier(ctx context.Context) (string, bool, error) {
	creds, err := p.credentialsFor(ctx)
	if errors.Is(err, errNoCredentials) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return creds.phoneNumberID, true, nil
}

// credentialsFor answers the calling tenant's credentials, the env seed for the default tenant.
func (p *Plugin) credentialsFor(ctx context.Context) (credentials, error) {
	phoneNumberID, encrypted, err := p.store.encryptedCredentials(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		if sdk.TenantOrDefault(ctx) == sdk.DefaultTenantID && p.envCredentials.complete() {
			return p.envCredentials, nil
		}
		return credentials{}, errNoCredentials
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

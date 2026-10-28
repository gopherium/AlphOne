// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// boundEncrypter ties each value to the calling tenant and purpose without a key, recording every call it answers.
type boundEncrypter struct {
	calls []string
}

// Encrypt answers the plaintext hex encoded behind the calling tenant and the purpose.
func (e *boundEncrypter) Encrypt(ctx context.Context, purpose string, plaintext []byte) ([]byte, error) {
	e.calls = append(e.calls, "Encrypt "+purpose)
	return append(boundPrefix(ctx, purpose), hex.EncodeToString(plaintext)...), nil
}

// Decrypt answers what Encrypt took, ErrCannotDecrypt for another tenant or purpose.
func (e *boundEncrypter) Decrypt(ctx context.Context, purpose string, ciphertext []byte) ([]byte, error) {
	e.calls = append(e.calls, "Decrypt "+purpose)
	encoded, bound := bytes.CutPrefix(ciphertext, boundPrefix(ctx, purpose))
	if !bound {
		return nil, sdk.ErrCannotDecrypt
	}
	return hex.DecodeString(string(encoded))
}

// boundPrefix answers the bytes that tie a value to the calling tenant and the purpose.
func boundPrefix(ctx context.Context, purpose string) []byte {
	return []byte(sdk.TenantOrDefault(ctx).String() + "/" + purpose + "/")
}

// keylessEncrypter is the encrypter of a host that holds no encryption key.
type keylessEncrypter struct{}

// Encrypt answers ErrNoEncryptionKey.
func (keylessEncrypter) Encrypt(context.Context, string, []byte) ([]byte, error) {
	return nil, sdk.ErrNoEncryptionKey
}

// Decrypt answers ErrNoEncryptionKey.
func (keylessEncrypter) Decrypt(context.Context, string, []byte) ([]byte, error) {
	return nil, sdk.ErrNoEncryptionKey
}

// newEncryptingPlugin returns a migrated plugin holding a bound encrypter.
func newEncryptingPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	p.UseEncrypter(&boundEncrypter{})
	return p
}

func TestCredentialsForReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}}

	_, err := p.credentialsFor(t.Context())

	if err == nil || errors.Is(err, errNoCredentials) {
		t.Errorf("credentialsFor() error = %v, want the connection failure surfaced", err)
	}
}

func TestATokenCopiedIntoAnotherTenantDoesNotOpenThere(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme, globex := seededTenant(t, p, "Acme"), seededTenant(t, p, "Globex")
	mine := sdk.WithTenant(t.Context(), acme)
	if err := p.SetCredentials(mine, "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_whatsapp.credentials (tenant_id, phone_number_id, access_token)"+
			" SELECT $1, '5550002', access_token FROM plugin_whatsapp.credentials WHERE tenant_id = $2",
		globex, acme); err != nil {
		t.Fatalf("copying the token: %v", err)
	}

	if _, err := p.credentialsFor(sdk.WithTenant(t.Context(), globex)); !errors.Is(err, sdk.ErrCannotDecrypt) {
		t.Errorf("credentialsFor() where the token was copied error = %v, want %v", err, sdk.ErrCannotDecrypt)
	}
	held, err := p.credentialsFor(mine)
	if want := (credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"}); err != nil || held != want {
		t.Errorf("credentialsFor() in its own tenant = %+v, %v, want %+v", held, err, want)
	}
}

func TestTheEncrypterIsAskedForTheAccessTokenPurpose(t *testing.T) {
	t.Parallel()

	encrypter := &boundEncrypter{}
	p := newMigratedPlugin(t)
	p.UseEncrypter(encrypter)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	if err := p.SetCredentials(mine, "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}
	if _, err := p.credentialsFor(mine); err != nil {
		t.Fatalf("credentialsFor() error = %v, want nil", err)
	}

	if want := []string{"Encrypt access_token", "Decrypt access_token"}; !slices.Equal(encrypter.calls, want) {
		t.Errorf("the encrypter answered %q, want %q", encrypter.calls, want)
	}
}

func TestWithoutAnEncryptionKeyATokenIsNeitherStoredNorOpened(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	p.UseEncrypter(keylessEncrypter{})
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))

	if err := p.SetCredentials(mine, "5550001", "EAAG-acme-token"); !errors.Is(err, sdk.ErrNoEncryptionKey) {
		t.Errorf("SetCredentials() error = %v, want %v", err, sdk.ErrNoEncryptionKey)
	}
	if held := tableCount(t, p, "plugin_whatsapp.credentials"); held != 0 {
		t.Errorf("the plugin holds %d tokens, want none stored without a key", held)
	}
	if err := p.store.upsertCredentials(mine, "5550001", []byte("encrypted under a key")); err != nil {
		t.Fatalf("upsertCredentials() error = %v, want nil", err)
	}
	if _, err := p.credentialsFor(mine); !errors.Is(err, sdk.ErrNoEncryptionKey) {
		t.Errorf("credentialsFor() of a stored token error = %v, want %v", err, sdk.ErrNoEncryptionKey)
	}
}

func TestThePluginNamesItsChannel(t *testing.T) {
	t.Parallel()

	if held := (&Plugin{}).Channel(); held != "whatsapp" {
		t.Errorf("Channel() = %q, want whatsapp", held)
	}
}

func TestChannelCredentialsRoundTripThroughTheSeam(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	mine := sdk.WithTenant(t.Context(), acme)

	if err := p.SetChannelCredentials(mine, "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
	}

	identifier, configured, err := p.ChannelIdentifier(mine)
	if err != nil {
		t.Fatalf("ChannelIdentifier() error = %v, want nil", err)
	}
	if !configured || identifier != "5550001" {
		t.Errorf("ChannelIdentifier() = (%q, %t), want the stored number reported", identifier, configured)
	}
}

func TestChannelIdentifierAnswersNothingWhenUnconfigured(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	bare := sdk.WithTenant(t.Context(), seededTenant(t, p, "Globex"))

	identifier, configured, err := p.ChannelIdentifier(bare)

	if err != nil {
		t.Fatalf("ChannelIdentifier() error = %v, want nil", err)
	}
	if configured || identifier != "" {
		t.Errorf("ChannelIdentifier() = (%q, %t), want nothing configured", identifier, configured)
	}
}

func TestChannelIdentifierAnswersTheEnvSeed(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}

	identifier, configured, err := p.ChannelIdentifier(t.Context())

	if err != nil {
		t.Fatalf("ChannelIdentifier() error = %v, want nil", err)
	}
	if !configured || identifier != "5550009" {
		t.Errorf("ChannelIdentifier() = (%q, %t), want the env number reported", identifier, configured)
	}
}

func TestChannelIdentifierReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}}

	if _, _, err := p.ChannelIdentifier(t.Context()); err == nil {
		t.Error("ChannelIdentifier() error = nil, want the connection failure surfaced")
	}
}

func TestCredentialsRoundTripEncryptedInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	mine := sdk.WithTenant(t.Context(), acme)

	if err := p.SetCredentials(mine, "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}

	held, err := p.credentialsFor(mine)
	if err != nil {
		t.Fatalf("credentialsFor() error = %v, want nil", err)
	}
	want := credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"}
	if held != want {
		t.Errorf("credentialsFor() = %+v, want %+v", held, want)
	}
	var atRest []byte
	if err := p.pool.QueryRow(t.Context(),
		"SELECT access_token FROM plugin_whatsapp.credentials WHERE tenant_id = $1", acme,
	).Scan(&atRest); err != nil {
		t.Fatalf("reading the stored token: %v", err)
	}
	if bytes.Contains(atRest, []byte("EAAG-acme-token")) {
		t.Error("the stored token is readable at rest, want it encrypted")
	}
}

func TestCredentialsStayInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	elsewhere := seededTenant(t, p, "Globex")
	if err := p.SetCredentials(sdk.WithTenant(t.Context(), acme), "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}

	if _, err := p.credentialsFor(sdk.WithTenant(t.Context(), elsewhere)); !errors.Is(err, errNoCredentials) {
		t.Errorf("credentialsFor() from another tenant error = %v, want errNoCredentials", err)
	}
}

func TestTheDefaultTenantAnswersTheEnvSeedWithOrWithoutAKey(t *testing.T) {
	t.Parallel()

	tests := map[string]sdk.Encrypter{
		"with a key":    &boundEncrypter{},
		"without a key": keylessEncrypter{},
	}
	for testName, encrypter := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			p.UseEncrypter(encrypter)
			p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}

			held, err := p.credentialsFor(t.Context())

			if err != nil {
				t.Fatalf("credentialsFor() error = %v, want nil", err)
			}
			if held != p.envCredentials {
				t.Errorf("credentialsFor() = %+v, want the env seed", held)
			}
		})
	}
}

func TestAStoredDefaultTokenThatDoesNotOpenAnswersTheError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		encrypter sdk.Encrypter
		want      error
	}{
		"without a key":                         {encrypter: keylessEncrypter{}, want: sdk.ErrNoEncryptionKey},
		"with a token encrypted somewhere else": {encrypter: &boundEncrypter{}, want: sdk.ErrCannotDecrypt},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			p.UseEncrypter(test.encrypter)
			p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
			if err := p.store.upsertCredentials(t.Context(), "5550001", []byte("encrypted somewhere else")); err != nil {
				t.Fatalf("upsertCredentials() error = %v, want nil", err)
			}

			held, err := p.credentialsFor(t.Context())

			if !errors.Is(err, test.want) {
				t.Errorf("credentialsFor() = %+v, %v, want %v rather than the env seed", held, err, test.want)
			}
		})
	}
}

func TestRoutingNamesTheTenantRatherThanInheritingIt(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	ambient := sdk.WithTenant(t.Context(), uuid.Must(uuid.NewV7()))

	tests := map[string]string{
		"an arrival naming no number":      "",
		"an arrival naming the env number": "5550009",
	}
	for testName, arriving := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			routed, known, err := p.routeByNumber(ambient, arriving)

			if err != nil {
				t.Fatalf("routeByNumber() error = %v, want nil", err)
			}
			if !known {
				t.Fatal("routeByNumber() dropped the arrival, want the configured number to own it")
			}
			if held := sdk.TenantOrDefault(routed); held != sdk.DefaultTenantID {
				t.Errorf("routed tenant = %s, want the default tenant rather than the ambient one", held)
			}
		})
	}
}

func TestAnUnconfiguredDefaultTenantHasNoCredentials(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)

	_, err := p.credentialsFor(t.Context())

	if !errors.Is(err, errNoCredentials) {
		t.Errorf("credentialsFor() with an empty environment error = %v, want errNoCredentials", err)
	}
}

func TestAHalfConfiguredDefaultTenantHasNoCredentials(t *testing.T) {
	t.Parallel()

	tests := map[string]credentials{
		"a number without a token": {phoneNumberID: "5550009"},
		"a token without a number": {accessToken: "EAAG-env-token"},
	}
	for testName, seed := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newEncryptingPlugin(t)
			p.envCredentials = seed

			_, err := p.credentialsFor(t.Context())

			if !errors.Is(err, errNoCredentials) {
				t.Errorf("credentialsFor() error = %v, want errNoCredentials", err)
			}
		})
	}
}

func TestASecondSetReplacesTheCredentials(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	mine := sdk.WithTenant(t.Context(), acme)
	if err := p.SetCredentials(mine, "5550001", "EAAG-old-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}

	if err := p.SetCredentials(mine, "5550002", "EAAG-new-token"); err != nil {
		t.Fatalf("SetCredentials() again error = %v, want nil", err)
	}

	held, err := p.credentialsFor(mine)
	if err != nil {
		t.Fatalf("credentialsFor() error = %v, want nil", err)
	}
	want := credentials{phoneNumberID: "5550002", accessToken: "EAAG-new-token"}
	if held != want {
		t.Errorf("credentialsFor() = %+v, want the replacement", held)
	}
}

func TestTwoTenantsMayNotShareOneNumber(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	elsewhere := seededTenant(t, p, "Globex")
	if err := p.SetCredentials(sdk.WithTenant(t.Context(), acme), "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}

	if err := p.SetCredentials(sdk.WithTenant(t.Context(), elsewhere), "5550001", "EAAG-globex-token"); err == nil {
		t.Error("SetCredentials() of a taken number answered, want the unique to refuse it")
	}
}

func TestTenantForPhoneNumberAnswersTheOwner(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	if err := p.SetCredentials(sdk.WithTenant(t.Context(), acme), "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}

	owner, err := p.store.tenantForPhoneNumber(t.Context(), "5550001")

	if err != nil {
		t.Fatalf("tenantForPhoneNumber() error = %v, want nil", err)
	}
	if owner != acme {
		t.Errorf("tenantForPhoneNumber() = %s, want the owning tenant", owner)
	}
}

func TestSetCredentialsDemandsBothValues(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)

	if err := p.SetCredentials(t.Context(), "", "EAAG-token"); err == nil {
		t.Error("SetCredentials() without a number answered, want it refused")
	}
	if err := p.SetCredentials(t.Context(), "5550001", ""); err == nil {
		t.Error("SetCredentials() without a token answered, want it refused")
	}
}

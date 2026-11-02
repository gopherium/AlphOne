// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"testing"

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

// failingSecretEncrypter encrypts a token and fails on anything else.
type failingSecretEncrypter struct {
	boundEncrypter
}

// Encrypt answers ErrNoEncryptionKey for every purpose but the access token.
func (e *failingSecretEncrypter) Encrypt(ctx context.Context, purpose string, plaintext []byte) ([]byte, error) {
	if purpose != accessTokenPurpose {
		return nil, sdk.ErrNoEncryptionKey
	}
	return e.boundEncrypter.Encrypt(ctx, purpose, plaintext)
}

// newEncryptingPlugin returns a migrated plugin holding a bound encrypter.
func newEncryptingPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	p.UseEncrypter(&boundEncrypter{})
	return p
}

// connected stores a number with its token alone in the tenant ctx serves, failing the test on an error.
func connected(t *testing.T, p *Plugin, ctx context.Context, held credentials) {
	t.Helper()
	stored := sdk.ChannelCredentials{Identifier: held.phoneNumberID, Secret: held.accessToken}
	if err := p.SetChannelCredentials(ctx, stored); err != nil {
		t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
	}
}

// fullyConnected stores the number 5550001 carrying every value of its own in the tenant ctx serves.
func fullyConnected(t *testing.T, p *Plugin, ctx context.Context) {
	t.Helper()
	if err := p.SetChannelCredentials(ctx, sdk.ChannelCredentials{
		Identifier:  "5550001",
		Label:       "Sales",
		Secret:      "EAAG-5550001",
		AppSecret:   "app-secret-5550001",
		VerifyToken: "verify-5550001",
	}); err != nil {
		t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
	}
}

// storedSecrets returns the app secret and the verify token digest of the only stored number.
func storedSecrets(t *testing.T, p *Plugin) ([]byte, []byte) {
	t.Helper()
	var appSecret, digest []byte
	if err := p.pool.QueryRow(t.Context(),
		"SELECT app_secret, verify_token_digest FROM plugin_whatsapp.credentials").Scan(&appSecret, &digest); err != nil {
		t.Fatalf("reading the stored secrets: %v", err)
	}
	return appSecret, digest
}

func TestATokenCopiedIntoAnotherTenantDoesNotOpenThere(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme, globex := seededTenant(t, p, "Acme"), seededTenant(t, p, "Globex")
	mine := sdk.WithTenant(t.Context(), acme)
	want := credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"}
	connected(t, p, mine, want)
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_whatsapp.credentials (tenant_id, phone_number_id, access_token)"+
			" SELECT $1, '5550002', access_token FROM plugin_whatsapp.credentials WHERE tenant_id = $2",
		globex, acme); err != nil {
		t.Fatalf("copying the token: %v", err)
	}

	if _, err := p.credentialsFor(sdk.WithTenant(t.Context(), globex), ""); !errors.Is(err, sdk.ErrCannotDecrypt) {
		t.Errorf("credentialsFor() where the token was copied error = %v, want %v", err, sdk.ErrCannotDecrypt)
	}
	held, err := p.credentialsFor(mine, "")
	if err != nil || held != want {
		t.Errorf("credentialsFor() in its own tenant = %+v, %v, want %+v", held, err, want)
	}
}

func TestTheEncrypterIsAskedForEachPurpose(t *testing.T) {
	t.Parallel()

	encrypter := &boundEncrypter{}
	p := newMigratedPlugin(t)
	p.UseEncrypter(encrypter)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	fullyConnected(t, p, mine)
	if _, err := p.credentialsFor(mine, "5550001"); err != nil {
		t.Fatalf("credentialsFor() error = %v, want nil", err)
	}

	want := []string{"Encrypt access_token", "Encrypt app_secret", "Decrypt access_token"}
	if !slices.Equal(encrypter.calls, want) {
		t.Errorf("the encrypter answered %q, want %q", encrypter.calls, want)
	}
}

func TestWithoutAnEncryptionKeyATokenIsNeitherStoredNorOpened(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	p.UseEncrypter(keylessEncrypter{})
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))

	err := p.SetChannelCredentials(mine, sdk.ChannelCredentials{Identifier: "5550001", Secret: "EAAG-acme-token"})

	if !errors.Is(err, sdk.ErrNoEncryptionKey) {
		t.Errorf("SetChannelCredentials() error = %v, want %v", err, sdk.ErrNoEncryptionKey)
	}
	if held := tableCount(t, p, "plugin_whatsapp.credentials"); held != 0 {
		t.Errorf("the plugin holds %d tokens, want none stored without a key", held)
	}
	stored := storedNumber{phoneNumberID: "5550001", accessToken: []byte("encrypted under a key")}
	if _, err := p.store.upsertCredentials(mine, stored); err != nil {
		t.Fatalf("upsertCredentials() error = %v, want nil", err)
	}
	if _, err := p.credentialsFor(mine, ""); !errors.Is(err, sdk.ErrNoEncryptionKey) {
		t.Errorf("credentialsFor() of a stored token error = %v, want %v", err, sdk.ErrNoEncryptionKey)
	}
}

func TestAnAppSecretIsNotStoredWithoutAnEncryptionKey(t *testing.T) {
	t.Parallel()

	encrypter := &failingSecretEncrypter{boundEncrypter: boundEncrypter{}}
	p := newMigratedPlugin(t)
	p.UseEncrypter(encrypter)

	err := p.SetChannelCredentials(t.Context(), sdk.ChannelCredentials{
		Identifier: "5550001", Secret: "EAAG-acme-token", AppSecret: "app-secret",
	})

	if !errors.Is(err, sdk.ErrNoEncryptionKey) {
		t.Errorf("SetChannelCredentials() error = %v, want %v", err, sdk.ErrNoEncryptionKey)
	}
	if held := tableCount(t, p, "plugin_whatsapp.credentials"); held != 0 {
		t.Errorf("the plugin holds %d numbers, want none stored without its secret", held)
	}
}

func TestThePluginNamesItsChannel(t *testing.T) {
	t.Parallel()

	if held := (&Plugin{}).Channel(); held != "whatsapp" {
		t.Errorf("Channel() = %q, want whatsapp", held)
	}
}

func TestCredentialsRoundTripEncryptedInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	mine := sdk.WithTenant(t.Context(), acme)
	want := credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"}
	connected(t, p, mine, want)

	held, err := p.credentialsFor(mine, "")
	if err != nil {
		t.Fatalf("credentialsFor() error = %v, want nil", err)
	}
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

func TestAppSecretAndVerifyTokenRestUnreadable(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	fullyConnected(t, p, mine)

	appSecret, digest := storedSecrets(t, p)

	if len(appSecret) == 0 || bytes.Contains(appSecret, []byte("app-secret-5550001")) {
		t.Errorf("the stored app secret = %q, want it held and encrypted", appSecret)
	}
	want := sha256.Sum256([]byte("verify-5550001"))
	if !bytes.Equal(digest, want[:]) {
		t.Errorf("the stored verify token = %x, want its SHA-256 %x", digest, want)
	}
}

func TestANumberStoredWithoutItsOwnSecretsKeepsThemEmpty(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	connected(t, p, t.Context(), credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"})

	appSecret, digest := storedSecrets(t, p)

	if appSecret != nil || digest != nil {
		t.Errorf("the stored secrets = (%x, %x), want both empty so the settings answer", appSecret, digest)
	}
}

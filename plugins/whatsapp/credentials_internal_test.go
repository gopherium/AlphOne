// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
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

// connected stores a number with its token alone in the tenant ctx serves, failing the test on an error.
func connected(t *testing.T, p *Plugin, ctx context.Context, number, token string) {
	t.Helper()
	if err := p.SetChannelCredentials(ctx, sdk.ChannelCredentials{Identifier: number, Secret: token}); err != nil {
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

func TestCredentialsForReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}}

	_, err := p.credentialsFor(t.Context(), "")

	if err == nil || errors.Is(err, errNoCredentials) {
		t.Errorf("credentialsFor() error = %v, want the connection failure surfaced", err)
	}
}

func TestATokenCopiedIntoAnotherTenantDoesNotOpenThere(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme, globex := seededTenant(t, p, "Acme"), seededTenant(t, p, "Globex")
	mine := sdk.WithTenant(t.Context(), acme)
	connected(t, p, mine, "5550001", "EAAG-acme-token")
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
	if want := (credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"}); err != nil || held != want {
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

	stored := sdk.ChannelCredentials{Identifier: "5550001", Label: "Sales", Secret: "EAAG-acme-token"}
	if err := p.SetChannelCredentials(mine, stored); err != nil {
		t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
	}

	identities, err := p.ChannelIdentities(mine)
	if err != nil {
		t.Fatalf("ChannelIdentities() error = %v, want nil", err)
	}
	if want := []sdk.ChannelIdentity{{Identifier: "5550001", Label: "Sales"}}; !slices.Equal(identities, want) {
		t.Errorf("ChannelIdentities() = %+v, want %+v", identities, want)
	}
}

func TestChannelIdentitiesAnswerNothingWhenUnconfigured(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	bare := sdk.WithTenant(t.Context(), seededTenant(t, p, "Globex"))

	identities, err := p.ChannelIdentities(bare)

	if err != nil {
		t.Fatalf("ChannelIdentities() error = %v, want nil", err)
	}
	if len(identities) != 0 {
		t.Errorf("ChannelIdentities() = %+v, want nothing configured", identities)
	}
}

func TestChannelIdentitiesAnswerTheEnvSeed(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}

	identities, err := p.ChannelIdentities(t.Context())

	if err != nil {
		t.Fatalf("ChannelIdentities() error = %v, want nil", err)
	}
	if want := []sdk.ChannelIdentity{{Identifier: "5550009"}}; !slices.Equal(identities, want) {
		t.Errorf("ChannelIdentities() = %+v, want the env number reported", identities)
	}
}

func TestChannelIdentitiesPreferWhatATenantStored(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	connected(t, p, t.Context(), "5550001", "EAAG-stored-token")

	identities, err := p.ChannelIdentities(t.Context())

	if err != nil {
		t.Fatalf("ChannelIdentities() error = %v, want nil", err)
	}
	if want := []sdk.ChannelIdentity{{Identifier: "5550001"}}; !slices.Equal(identities, want) {
		t.Errorf("ChannelIdentities() = %+v, want the stored number alone", identities)
	}
}

func TestChannelIdentitiesLeaveTheEnvSeedToTheDefaultTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	acme := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))

	identities, err := p.ChannelIdentities(acme)

	if err != nil {
		t.Fatalf("ChannelIdentities() error = %v, want nil", err)
	}
	if len(identities) != 0 {
		t.Errorf("ChannelIdentities() = %+v from another tenant, want the env number withheld", identities)
	}
}

func TestChannelIdentitiesOpenNoSecret(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	encrypter := &boundEncrypter{}
	p.UseEncrypter(encrypter)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	fullyConnected(t, p, mine)
	encrypter.calls = nil

	if _, err := p.ChannelIdentities(mine); err != nil {
		t.Fatalf("ChannelIdentities() error = %v, want nil", err)
	}

	if len(encrypter.calls) != 0 {
		t.Errorf("listing the numbers asked the encrypter for %q, want nothing opened", encrypter.calls)
	}
}

func TestChannelIdentitiesReportAStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}}

	if _, err := p.ChannelIdentities(t.Context()); err == nil {
		t.Error("ChannelIdentities() error = nil, want the connection failure surfaced")
	}
}

func TestCredentialsRoundTripEncryptedInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	mine := sdk.WithTenant(t.Context(), acme)
	connected(t, p, mine, "5550001", "EAAG-acme-token")

	held, err := p.credentialsFor(mine, "")
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

func TestAppSecretAndVerifyTokenRestUnreadable(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	fullyConnected(t, p, mine)

	var appSecret, digest []byte
	if err := p.pool.QueryRow(t.Context(),
		"SELECT app_secret, verify_token_digest FROM plugin_whatsapp.credentials").Scan(&appSecret, &digest); err != nil {
		t.Fatalf("reading the stored secrets: %v", err)
	}

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
	connected(t, p, t.Context(), "5550001", "EAAG-acme-token")

	var appSecret, digest []byte
	if err := p.pool.QueryRow(t.Context(),
		"SELECT app_secret, verify_token_digest FROM plugin_whatsapp.credentials").Scan(&appSecret, &digest); err != nil {
		t.Fatalf("reading the stored secrets: %v", err)
	}

	if appSecret != nil || digest != nil {
		t.Errorf("the stored secrets = (%x, %x), want both empty so the settings answer", appSecret, digest)
	}
}

func TestCredentialsStayInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	elsewhere := seededTenant(t, p, "Globex")
	connected(t, p, sdk.WithTenant(t.Context(), acme), "5550001", "EAAG-acme-token")

	if _, err := p.credentialsFor(sdk.WithTenant(t.Context(), elsewhere), ""); !errors.Is(err, errNoCredentials) {
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

			held, err := p.credentialsFor(t.Context(), "")

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
			stored := storedNumber{phoneNumberID: "5550009", accessToken: []byte("encrypted somewhere else")}
			if _, err := p.store.upsertCredentials(t.Context(), stored); err != nil {
				t.Fatalf("upsertCredentials() error = %v, want nil", err)
			}

			held, err := p.credentialsFor(t.Context(), "")

			if !errors.Is(err, test.want) {
				t.Errorf("credentialsFor() = %+v, %v, want %v rather than the env seed", held, err, test.want)
			}
		})
	}
}

func TestAnEmptyNumberAnswersTheEnvironmentBesideAnotherStoredNumber(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	connected(t, p, t.Context(), "5550001", "EAAG-stored-token")

	held, err := p.credentialsFor(t.Context(), "")

	if err != nil || held != p.envCredentials {
		t.Errorf("credentialsFor() = %+v, %v, want the environment credentials", held, err)
	}
}

func TestAnEmptyNumberAnswersTheRowOfTheEnvironmentNumber(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	connected(t, p, t.Context(), "5550001", "EAAG-older-token")
	connected(t, p, t.Context(), "5550009", "EAAG-stored-token")

	held, err := p.credentialsFor(t.Context(), "")

	if want := (credentials{phoneNumberID: "5550009", accessToken: "EAAG-stored-token"}); err != nil || held != want {
		t.Errorf("credentialsFor() = %+v, %v, want the row of the environment number %+v", held, err, want)
	}
}

func TestTheEnvironmentNumberAnswersItsCredentialsOnceItsRowIsRemoved(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	connected(t, p, t.Context(), "5550009", "EAAG-stored-token")
	if err := p.RemoveChannelCredentials(t.Context(), "5550009"); err != nil {
		t.Fatalf("RemoveChannelCredentials() error = %v, want nil", err)
	}

	for testName, number := range map[string]string{"an empty number": "", "the given number": "5550009"} {
		t.Run(testName, func(t *testing.T) {
			held, err := p.credentialsFor(t.Context(), number)

			if err != nil || held != p.envCredentials {
				t.Errorf("credentialsFor(%q) = %+v, %v, want the environment credentials", number, held, err)
			}
		})
	}
}

func TestAnotherTenantNeverReceivesTheEnvironmentCredentials(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	bare := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))

	tests := map[string]struct {
		number string
		want   error
	}{
		"the environment number": {number: "5550009", want: errNumberRemoved},
		"an empty number":        {number: "", want: errNoCredentials},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			held, err := p.credentialsFor(bare, test.number)

			if !errors.Is(err, test.want) || held != (credentials{}) {
				t.Errorf("credentialsFor(%q) = %+v, %v, want nothing and %v", test.number, held, err, test.want)
			}
		})
	}
}

func TestAnEnvironmentNumberWithoutATokenAnswersNoOtherStoredNumber(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009"}
	connected(t, p, t.Context(), "5550001", "EAAG-stored-token")

	for testName, number := range map[string]string{"an empty number": "", "the given number": "5550009"} {
		t.Run(testName, func(t *testing.T) {
			held, err := p.credentialsFor(t.Context(), number)

			if !errors.Is(err, errNoCredentials) || held != (credentials{}) {
				t.Errorf("credentialsFor(%q) = %+v, %v, want nothing and errNoCredentials", number, held, err)
			}
		})
	}
}

func TestAnEnvironmentNumberWithoutATokenAnswersItsOwnRow(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009"}
	connected(t, p, t.Context(), "5550001", "EAAG-older-token")
	connected(t, p, t.Context(), "5550009", "EAAG-stored-token")

	held, err := p.credentialsFor(t.Context(), "")

	if want := (credentials{phoneNumberID: "5550009", accessToken: "EAAG-stored-token"}); err != nil || held != want {
		t.Errorf("credentialsFor() = %+v, %v, want the row of the environment number %+v", held, err, want)
	}
}

func TestWithoutAnEnvironmentNumberTheDefaultTenantAnswersItsOldest(t *testing.T) {
	t.Parallel()

	tests := map[string]credentials{
		"no environment number":    {},
		"a token without a number": {accessToken: "EAAG-env-token"},
	}
	for testName, seed := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newEncryptingPlugin(t)
			p.envCredentials = seed
			connected(t, p, t.Context(), "5550002", "EAAG-older-token")
			connected(t, p, t.Context(), "5550001", "EAAG-newer-token")

			held, err := p.credentialsFor(t.Context(), "")

			if want := (credentials{phoneNumberID: "5550002", accessToken: "EAAG-older-token"}); err != nil || held != want {
				t.Errorf("credentialsFor() = %+v, %v, want the oldest number %+v", held, err, want)
			}
			if _, err := p.credentialsFor(t.Context(), "5550009"); !errors.Is(err, errNumberRemoved) {
				t.Errorf("credentialsFor() of a number with no row error = %v, want errNumberRemoved", err)
			}
		})
	}
}

func TestAnUnconfiguredDefaultTenantHasNoCredentials(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)

	_, err := p.credentialsFor(t.Context(), "")

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

			_, err := p.credentialsFor(t.Context(), "")

			if !errors.Is(err, errNoCredentials) {
				t.Errorf("credentialsFor() error = %v, want errNoCredentials", err)
			}
		})
	}
}

func TestASecondNumberJoinsTheFirst(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	connected(t, p, mine, "5550001", "EAAG-first-token")

	connected(t, p, mine, "5550002", "EAAG-second-token")

	identities, err := p.ChannelIdentities(mine)
	if err != nil {
		t.Fatalf("ChannelIdentities() error = %v, want nil", err)
	}
	if want := []sdk.ChannelIdentity{{Identifier: "5550001"}, {Identifier: "5550002"}}; !slices.Equal(identities, want) {
		t.Errorf("ChannelIdentities() = %+v, want both numbers, oldest first", identities)
	}
	for number, token := range map[string]string{"5550001": "EAAG-first-token", "5550002": "EAAG-second-token"} {
		held, err := p.credentialsFor(mine, number)
		if want := (credentials{phoneNumberID: number, accessToken: token}); err != nil || held != want {
			t.Errorf("credentialsFor(%s) = %+v, %v, want %+v", number, held, err, want)
		}
	}
}

func TestAnEmptyNumberAnswersTheTenantsOldestNumber(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	connected(t, p, mine, "5550002", "EAAG-older-token")
	connected(t, p, mine, "5550001", "EAAG-newer-token")

	held, err := p.credentialsFor(mine, "")

	if want := (credentials{phoneNumberID: "5550002", accessToken: "EAAG-older-token"}); err != nil || held != want {
		t.Errorf("credentialsFor() = %+v, %v, want the oldest number %+v", held, err, want)
	}
}

func TestAGivenNumberAnswersOnlyInsideItsTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	acme := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	elsewhere := sdk.WithTenant(t.Context(), seededTenant(t, p, "Globex"))
	connected(t, p, acme, "5550001", "EAAG-acme-token")

	tests := map[string]struct {
		ctx    context.Context
		number string
	}{
		"a number never stored":                  {ctx: acme, number: "5550002"},
		"a number another tenant holds":          {ctx: elsewhere, number: "5550001"},
		"a number the default tenant never held": {ctx: t.Context(), number: "5550001"},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			if _, err := p.credentialsFor(test.ctx, test.number); !errors.Is(err, errNumberRemoved) {
				t.Errorf("credentialsFor() error = %v, want errNumberRemoved", err)
			}
		})
	}
}

func TestReconnectingANumberReplacesItsTokenAndKeepsTheRest(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	fullyConnected(t, p, mine)
	var before, digestBefore []byte
	if err := p.pool.QueryRow(t.Context(),
		"SELECT app_secret, verify_token_digest FROM plugin_whatsapp.credentials").Scan(&before, &digestBefore); err != nil {
		t.Fatalf("reading the stored secrets: %v", err)
	}

	connected(t, p, mine, "5550001", "EAAG-replaced-token")

	held, err := p.credentialsFor(mine, "5550001")
	if want := (credentials{phoneNumberID: "5550001", accessToken: "EAAG-replaced-token"}); err != nil || held != want {
		t.Errorf("credentialsFor() = %+v, %v, want %+v", held, err, want)
	}
	identities, err := p.ChannelIdentities(mine)
	wantKept := []sdk.ChannelIdentity{{Identifier: "5550001", Label: "Sales"}}
	if err != nil || !slices.Equal(identities, wantKept) {
		t.Errorf("ChannelIdentities() = %+v, %v, want the label kept", identities, err)
	}
	var after, digestAfter []byte
	if err := p.pool.QueryRow(t.Context(),
		"SELECT app_secret, verify_token_digest FROM plugin_whatsapp.credentials").Scan(&after, &digestAfter); err != nil {
		t.Fatalf("reading the stored secrets: %v", err)
	}
	if !bytes.Equal(after, before) || !bytes.Equal(digestAfter, digestBefore) {
		t.Errorf("the stored secrets moved to (%x, %x), want them kept", after, digestAfter)
	}
}

func TestReconnectingANumberCanChangeEveryValue(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	fullyConnected(t, p, mine)

	if err := p.SetChannelCredentials(mine, sdk.ChannelCredentials{
		Identifier: "5550001", Label: "Support", Secret: "EAAG-replaced-token",
		AppSecret: "app-secret-replaced", VerifyToken: "verify-replaced",
	}); err != nil {
		t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
	}

	identities, err := p.ChannelIdentities(mine)
	wantRelabelled := []sdk.ChannelIdentity{{Identifier: "5550001", Label: "Support"}}
	if err != nil || !slices.Equal(identities, wantRelabelled) {
		t.Errorf("ChannelIdentities() = %+v, %v, want the new label", identities, err)
	}
	var appSecret, digest []byte
	if err := p.pool.QueryRow(t.Context(),
		"SELECT app_secret, verify_token_digest FROM plugin_whatsapp.credentials").Scan(&appSecret, &digest); err != nil {
		t.Fatalf("reading the stored secrets: %v", err)
	}
	opened, err := p.encrypter.Decrypt(mine, appSecretPurpose, appSecret)
	if err != nil || string(opened) != "app-secret-replaced" {
		t.Errorf("the app secret opens to %q, %v, want the new one", opened, err)
	}
	if want := sha256.Sum256([]byte("verify-replaced")); !bytes.Equal(digest, want[:]) {
		t.Errorf("the stored verify token = %x, want the new one", digest)
	}
}

func TestANumberAnotherTenantHoldsIsTurnedDownNamingNoTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	elsewhere := sdk.WithTenant(t.Context(), seededTenant(t, p, "Globex"))
	connected(t, p, sdk.WithTenant(t.Context(), acme), "5550001", "EAAG-acme-token")

	err := p.SetChannelCredentials(elsewhere, sdk.ChannelCredentials{
		Identifier: "5550001", Secret: "EAAG-globex-token", AppSecret: "app-secret-globex",
	})

	var refused sdk.GraphError
	if !errors.As(err, &refused) || refused.Code != "CONFLICT" || refused.Reason != "channel_number_taken" {
		t.Fatalf("SetChannelCredentials() error = %v, want the reason channel_number_taken", err)
	}
	said := fmt.Sprintf("%+v", err)
	for _, forbidden := range []string{acme.String(), "Acme", "app-secret-globex", "EAAG-globex-token"} {
		if strings.Contains(said, forbidden) {
			t.Errorf("the error says %q, want it to leave out %q", said, forbidden)
		}
	}
	held, err := p.credentialsFor(sdk.WithTenant(t.Context(), acme), "5550001")
	if err != nil || held.accessToken != "EAAG-acme-token" {
		t.Errorf("the holding tenant's token = %q, %v, want it untouched", held.accessToken, err)
	}
}

func TestAWorkspaceHoldingTheEnvironmentNumberReplacesItsToken(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	connected(t, p, mine, "5550009", "EAAG-first-token")
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}

	connected(t, p, mine, "5550009", "EAAG-replaced-token")

	held, err := p.credentialsFor(mine, "5550009")
	if want := (credentials{phoneNumberID: "5550009", accessToken: "EAAG-replaced-token"}); err != nil || held != want {
		t.Errorf("credentialsFor() = %+v, %v, want %+v", held, err, want)
	}
}

func TestAWorkspaceNotHoldingTheEnvironmentNumberIsTurnedDownBesideAHolder(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	elsewhere := sdk.WithTenant(t.Context(), seededTenant(t, p, "Globex"))
	connected(t, p, acme, "5550009", "EAAG-first-token")
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}

	err := p.SetChannelCredentials(elsewhere, sdk.ChannelCredentials{Identifier: "5550009", Secret: "EAAG-other-token"})

	var refused sdk.GraphError
	if !errors.As(err, &refused) || refused.Reason != "channel_number_taken" {
		t.Errorf("SetChannelCredentials() error = %v, want the reason channel_number_taken", err)
	}
}

func TestConnectingTheEnvironmentNumberReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{
		store:          &store{pool: newUnreachablePool(t)},
		encrypter:      &boundEncrypter{},
		envCredentials: credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"},
	}
	elsewhere := sdk.WithTenant(t.Context(), uuid.Must(uuid.NewV7()))

	err := p.SetChannelCredentials(elsewhere, sdk.ChannelCredentials{Identifier: "5550009", Secret: "EAAG-token"})

	var coded sdk.GraphError
	if err == nil || errors.As(err, &coded) {
		t.Errorf("SetChannelCredentials() error = %v, want a plain failure, not a taken number", err)
	}
}

func TestRemovingANumberLeavesTheOthers(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	connected(t, p, mine, "5550001", "EAAG-first-token")
	connected(t, p, mine, "5550002", "EAAG-second-token")

	if err := p.RemoveChannelCredentials(mine, "5550001"); err != nil {
		t.Fatalf("RemoveChannelCredentials() error = %v, want nil", err)
	}

	identities, err := p.ChannelIdentities(mine)
	if want := []sdk.ChannelIdentity{{Identifier: "5550002"}}; err != nil || !slices.Equal(identities, want) {
		t.Errorf("ChannelIdentities() = %+v, %v, want the other number alone", identities, err)
	}
	if _, err := p.credentialsFor(mine, "5550001"); !errors.Is(err, errNumberRemoved) {
		t.Errorf("credentialsFor() of the removed number error = %v, want errNumberRemoved", err)
	}
}

func TestRemovingANumberStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	elsewhere := sdk.WithTenant(t.Context(), seededTenant(t, p, "Globex"))
	connected(t, p, acme, "5550001", "EAAG-acme-token")

	if err := p.RemoveChannelCredentials(elsewhere, "5550001"); err != nil {
		t.Fatalf("RemoveChannelCredentials() error = %v, want the same answer as for a number never held", err)
	}

	if _, err := p.credentialsFor(acme, "5550001"); err != nil {
		t.Errorf("credentialsFor() in the holding tenant error = %v, want the number kept", err)
	}
}

func TestRemovingANumberReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}}

	if err := p.RemoveChannelCredentials(t.Context(), "5550001"); err == nil {
		t.Error("RemoveChannelCredentials() error = nil, want the connection failure surfaced")
	}
}

func TestSetChannelCredentialsDemandsANumberAndAToken(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)

	if err := p.SetChannelCredentials(t.Context(), sdk.ChannelCredentials{Secret: "EAAG-token"}); err == nil {
		t.Error("SetChannelCredentials() without a number answered, want it refused")
	}
	if err := p.SetChannelCredentials(t.Context(), sdk.ChannelCredentials{Identifier: "5550001"}); err == nil {
		t.Error("SetChannelCredentials() without a token answered, want it refused")
	}
}

func TestSetChannelCredentialsReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}, encrypter: &boundEncrypter{}}
	stored := sdk.ChannelCredentials{Identifier: "5550001", Secret: "EAAG-token", AppSecret: "app-secret"}

	err := p.SetChannelCredentials(t.Context(), stored)

	if err == nil {
		t.Fatal("SetChannelCredentials() error = nil, want the connection failure surfaced")
	}
	var coded sdk.GraphError
	if errors.As(err, &coded) {
		t.Errorf("SetChannelCredentials() error = %+v, want a plain failure, not a taken number", coded)
	}
	if said := err.Error(); strings.Contains(said, "EAAG-token") || strings.Contains(said, "app-secret") {
		t.Errorf("the error says %q, want no secret in it", said)
	}
}

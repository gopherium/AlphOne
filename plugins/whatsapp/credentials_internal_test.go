// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

// testCredentialsKey returns a parsed 32 byte key for sealing tests.
func testCredentialsKey(t *testing.T) []byte {
	t.Helper()
	key, err := credentialsKey(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("credentialsKey() error = %v, want nil", err)
	}
	return key
}

func TestASealedTokenRoundTrips(t *testing.T) {
	t.Parallel()

	key := testCredentialsKey(t)

	sealed, err := sealToken(key, "EAAG-token")
	if err != nil {
		t.Fatalf("sealToken() error = %v, want nil", err)
	}
	if bytes.Contains(sealed, []byte("EAAG-token")) {
		t.Error("the sealed bytes carry the token in the clear, want ciphertext")
	}
	opened, err := openToken(key, sealed)
	if err != nil {
		t.Fatalf("openToken() error = %v, want nil", err)
	}
	if opened != "EAAG-token" {
		t.Errorf("openToken() = %q, want the sealed token back", opened)
	}
}

func TestAnotherKeyCannotOpenASealedToken(t *testing.T) {
	t.Parallel()

	sealed, err := sealToken(testCredentialsKey(t), "EAAG-token")
	if err != nil {
		t.Fatalf("sealToken() error = %v, want nil", err)
	}
	otherKey, err := credentialsKey(strings.Repeat("cd", 32))
	if err != nil {
		t.Fatalf("credentialsKey() error = %v, want nil", err)
	}

	if _, err := openToken(otherKey, sealed); err == nil {
		t.Error("openToken() with another key answered, want it refused")
	}
}

func TestATamperedTokenIsRefused(t *testing.T) {
	t.Parallel()

	key := testCredentialsKey(t)
	sealed, err := sealToken(key, "EAAG-token")
	if err != nil {
		t.Fatalf("sealToken() error = %v, want nil", err)
	}
	sealed[len(sealed)-1] ^= 1

	if _, err := openToken(key, sealed); err == nil {
		t.Error("openToken() of tampered bytes answered, want it refused")
	}
}

func TestATruncatedSealIsRefused(t *testing.T) {
	t.Parallel()

	if _, err := openToken(testCredentialsKey(t), []byte("short")); err == nil {
		t.Error("openToken() of a truncated seal answered, want it refused")
	}
}

func TestCredentialsKeyDemandsSixtyFourHexCharacters(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw     string
		wantErr bool
		wantNil bool
	}{
		"a valid key": {raw: strings.Repeat("ab", 32)},
		"no key":      {raw: "", wantNil: true},
		"a short key": {raw: "abcd", wantErr: true},
		"not hex":     {raw: strings.Repeat("zz", 32), wantErr: true},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			key, err := credentialsKey(tc.raw)

			if (err != nil) != tc.wantErr {
				t.Fatalf("credentialsKey() error = %v, wantErr %t", err, tc.wantErr)
			}
			if tc.wantNil && key != nil {
				t.Errorf("credentialsKey() = %x, want nil for an empty value", key)
			}
			if !tc.wantErr && !tc.wantNil && len(key) != 32 {
				t.Errorf("len(key) = %d, want 32", len(key))
			}
		})
	}
}

func TestSealingWithoutAKeyIsRefused(t *testing.T) {
	t.Parallel()

	if _, err := sealToken(nil, "EAAG-token"); err == nil {
		t.Error("sealToken() without a key answered, want it refused")
	}
	if _, err := openToken(nil, []byte("sealed")); err == nil {
		t.Error("openToken() without a key answered, want it refused")
	}
}

func TestSealingWithAWrongSizedKeyIsRefused(t *testing.T) {
	t.Parallel()

	if _, err := sealToken([]byte("short"), "EAAG-token"); err == nil {
		t.Error("sealToken() with a wrong sized key answered, want it refused")
	}
}

func TestSealTokenReportsEntropyFailure(t *testing.T) {
	randRead = failingEntropy{}.Read
	defer func() { randRead = defaultRandRead }()

	if _, err := sealToken(testCredentialsKey(t), "EAAG-token"); !errors.Is(err, errEntropy) {
		t.Errorf("sealToken() error = %v, want the entropy failure in its chain", err)
	}
}

// degenerateBlock is a cipher block whose size no sealer accepts.
type degenerateBlock struct{}

// BlockSize reports a size GCM refuses.
func (degenerateBlock) BlockSize() int { return 5 }

// Encrypt does nothing.
func (degenerateBlock) Encrypt(_, _ []byte) {}

// Decrypt does nothing.
func (degenerateBlock) Decrypt(_, _ []byte) {}

func TestMustGCMRefusesADegenerateBlock(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("mustGCM() of a degenerate block returned, want a panic")
		}
	}()

	mustGCM(degenerateBlock{})
}

func TestCredentialsForReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}, key: testCredentialsKey(t)}

	_, err := p.credentialsFor(t.Context())

	if err == nil || errors.Is(err, errNoCredentials) {
		t.Errorf("credentialsFor() error = %v, want the connection failure surfaced", err)
	}
}

func TestCredentialsForRefusesATokenSealedUnderAnotherKey(t *testing.T) {
	t.Parallel()

	p := newSealingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	mine := sdk.WithTenant(t.Context(), acme)
	if err := p.SetCredentials(mine, "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}
	otherKey, err := credentialsKey(strings.Repeat("cd", 32))
	if err != nil {
		t.Fatalf("credentialsKey() error = %v, want nil", err)
	}
	p.key = otherKey

	if _, err := p.credentialsFor(mine); err == nil {
		t.Error("credentialsFor() under another key answered, want it refused")
	}
}

// newSealingPlugin returns a migrated plugin holding the test sealing key.
func newSealingPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	p.key = testCredentialsKey(t)
	return p
}

func TestCredentialsRoundTripSealedInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newSealingPlugin(t)
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
		t.Error("the stored token is readable at rest, want it sealed")
	}
}

func TestCredentialsStayInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newSealingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	elsewhere := seededTenant(t, p, "Globex")
	if err := p.SetCredentials(sdk.WithTenant(t.Context(), acme), "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}

	if _, err := p.credentialsFor(sdk.WithTenant(t.Context(), elsewhere)); !errors.Is(err, errNoCredentials) {
		t.Errorf("credentialsFor() from another tenant error = %v, want errNoCredentials", err)
	}
}

func TestTheDefaultTenantAnswersTheEnvSeed(t *testing.T) {
	t.Parallel()

	p := newSealingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}

	held, err := p.credentialsFor(t.Context())

	if err != nil {
		t.Fatalf("credentialsFor() error = %v, want nil", err)
	}
	if held != p.envCredentials {
		t.Errorf("credentialsFor() = %+v, want the env seed", held)
	}
}

func TestASecondSetReplacesTheCredentials(t *testing.T) {
	t.Parallel()

	p := newSealingPlugin(t)
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

	p := newSealingPlugin(t)
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

	p := newSealingPlugin(t)
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

func TestSetCredentialsWithoutAKeyIsRefused(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	if err := p.SetCredentials(t.Context(), "5550001", "EAAG-token"); !errors.Is(err, errNoCredentialsKey) {
		t.Errorf("SetCredentials() without a key error = %v, want errNoCredentialsKey", err)
	}
}

func TestSetCredentialsDemandsBothValues(t *testing.T) {
	t.Parallel()

	p := newSealingPlugin(t)

	if err := p.SetCredentials(t.Context(), "", "EAAG-token"); err == nil {
		t.Error("SetCredentials() without a number answered, want it refused")
	}
	if err := p.SetCredentials(t.Context(), "5550001", ""); err == nil {
		t.Error("SetCredentials() without a token answered, want it refused")
	}
}

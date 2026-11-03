// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"errors"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

// newEnvNumberPlugin returns a plugin that stores numbers and serves the number 5550009 from the environment.
func newEnvNumberPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	return p
}

// wantEmptyNumberAnswers fails the test unless an empty number answers want in the tenant ctx serves.
func wantEmptyNumberAnswers(t *testing.T, p *Plugin, ctx context.Context, want credentials) {
	t.Helper()
	if held, err := p.credentialsFor(ctx, ""); err != nil || held != want {
		t.Errorf("credentialsFor() = %+v, %v, want %+v", held, err, want)
	}
}

// wantNoCredentials fails the test unless an empty number answers errNoCredentials in the tenant ctx serves.
func wantNoCredentials(t *testing.T, p *Plugin, ctx context.Context) {
	t.Helper()
	if _, err := p.credentialsFor(ctx, ""); !errors.Is(err, errNoCredentials) {
		t.Errorf("credentialsFor() error = %v, want errNoCredentials", err)
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

func TestCredentialsStayInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := seededTenant(t, p, "Acme")
	elsewhere := seededTenant(t, p, "Globex")
	connected(t, p, sdk.WithTenant(t.Context(), acme),
		credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"})

	wantNoCredentials(t, p, sdk.WithTenant(t.Context(), elsewhere))
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

	p := newEnvNumberPlugin(t)
	connected(t, p, t.Context(), credentials{phoneNumberID: "5550001", accessToken: "EAAG-stored-token"})

	wantEmptyNumberAnswers(t, p, t.Context(), p.envCredentials)
}

func TestAnEmptyNumberAnswersTheRowOfTheEnvironmentNumber(t *testing.T) {
	t.Parallel()

	tests := map[string]credentials{
		"an environment number with a token":    {phoneNumberID: "5550009", accessToken: "EAAG-env-token"},
		"an environment number without a token": {phoneNumberID: "5550009"},
	}
	for testName, seed := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newEncryptingPlugin(t)
			p.envCredentials = seed
			row := credentials{phoneNumberID: "5550009", accessToken: "EAAG-stored-token"}
			connected(t, p, t.Context(), credentials{phoneNumberID: "5550001", accessToken: "EAAG-older-token"})
			connected(t, p, t.Context(), row)

			wantEmptyNumberAnswers(t, p, t.Context(), row)
		})
	}
}

func TestTheEnvironmentNumberAnswersItsCredentialsOnceItsRowIsRemoved(t *testing.T) {
	t.Parallel()

	p := newEnvNumberPlugin(t)
	connected(t, p, t.Context(), credentials{phoneNumberID: "5550009", accessToken: "EAAG-stored-token"})
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

	p := newEnvNumberPlugin(t)
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
	connected(t, p, t.Context(), credentials{phoneNumberID: "5550001", accessToken: "EAAG-stored-token"})

	for testName, number := range map[string]string{"an empty number": "", "the given number": "5550009"} {
		t.Run(testName, func(t *testing.T) {
			held, err := p.credentialsFor(t.Context(), number)

			if !errors.Is(err, errNoCredentials) || held != (credentials{}) {
				t.Errorf("credentialsFor(%q) = %+v, %v, want nothing and errNoCredentials", number, held, err)
			}
		})
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
			oldest := credentials{phoneNumberID: "5550002", accessToken: "EAAG-older-token"}
			connected(t, p, t.Context(), oldest)
			connected(t, p, t.Context(), credentials{phoneNumberID: "5550001", accessToken: "EAAG-newer-token"})

			wantEmptyNumberAnswers(t, p, t.Context(), oldest)
			if _, err := p.credentialsFor(t.Context(), "5550009"); !errors.Is(err, errNumberRemoved) {
				t.Errorf("credentialsFor() of a number with no row error = %v, want errNumberRemoved", err)
			}
		})
	}
}

func TestAnUnconfiguredDefaultTenantHasNoCredentials(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)

	wantNoCredentials(t, p, t.Context())
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

			wantNoCredentials(t, p, t.Context())
		})
	}
}

func TestAnEmptyNumberAnswersTheTenantsOldestNumber(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	oldest := credentials{phoneNumberID: "5550002", accessToken: "EAAG-older-token"}
	connected(t, p, mine, oldest)
	connected(t, p, mine, credentials{phoneNumberID: "5550001", accessToken: "EAAG-newer-token"})

	wantEmptyNumberAnswers(t, p, mine, oldest)
}

func TestAGivenNumberAnswersOnlyInsideItsTenant(t *testing.T) {
	t.Parallel()

	p := newEnvNumberPlugin(t)
	acme := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	elsewhere := sdk.WithTenant(t.Context(), seededTenant(t, p, "Globex"))
	connected(t, p, acme, credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"})

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

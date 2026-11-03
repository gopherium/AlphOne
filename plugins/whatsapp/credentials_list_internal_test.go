// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"slices"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

// identitiesOf returns the numbers the tenant ctx serves lists, failing the test on an error.
func identitiesOf(t *testing.T, p *Plugin, ctx context.Context) []sdk.ChannelIdentity {
	t.Helper()
	identities, err := p.ChannelIdentities(ctx)
	if err != nil {
		t.Fatalf("ChannelIdentities() error = %v, want nil", err)
	}
	return identities
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

	identities := identitiesOf(t, p, mine)

	if want := []sdk.ChannelIdentity{{Identifier: "5550001", Label: "Sales"}}; !slices.Equal(identities, want) {
		t.Errorf("ChannelIdentities() = %+v, want %+v", identities, want)
	}
}

func TestChannelIdentitiesAnswerNothingWhenUnconfigured(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	bare := sdk.WithTenant(t.Context(), seededTenant(t, p, "Globex"))

	identities := identitiesOf(t, p, bare)

	if len(identities) != 0 {
		t.Errorf("ChannelIdentities() = %+v, want nothing configured", identities)
	}
}

func TestChannelIdentitiesAnswerTheEnvSeed(t *testing.T) {
	t.Parallel()

	p := newEnvNumberPlugin(t)

	identities := identitiesOf(t, p, t.Context())

	if want := []sdk.ChannelIdentity{{Identifier: "5550009"}}; !slices.Equal(identities, want) {
		t.Errorf("ChannelIdentities() = %+v, want the env number reported", identities)
	}
}

func TestChannelIdentitiesPutTheEnvNumberBeforeWhatATenantStored(t *testing.T) {
	t.Parallel()

	p := newEnvNumberPlugin(t)
	connected(t, p, t.Context(), credentials{phoneNumberID: "5550002", accessToken: "EAAG-older-token"})
	connected(t, p, t.Context(), credentials{phoneNumberID: "5550001", accessToken: "EAAG-newer-token"})

	identities := identitiesOf(t, p, t.Context())

	want := []sdk.ChannelIdentity{{Identifier: "5550009"}, {Identifier: "5550002"}, {Identifier: "5550001"}}
	if !slices.Equal(identities, want) {
		t.Errorf("ChannelIdentities() = %+v, want the env number, then the stored ones oldest first", identities)
	}
}

func TestChannelIdentitiesKeepStoredOrderForAStoredEnvNumber(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		stored []string
		want   []sdk.ChannelIdentity
	}{
		"stored first": {
			stored: []string{"5550009", "5550001"},
			want:   []sdk.ChannelIdentity{{Identifier: "5550009"}, {Identifier: "5550001"}},
		},
		"stored last": {
			stored: []string{"5550001", "5550009"},
			want:   []sdk.ChannelIdentity{{Identifier: "5550001"}, {Identifier: "5550009"}},
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newEnvNumberPlugin(t)
			for _, number := range test.stored {
				connected(t, p, t.Context(), credentials{phoneNumberID: number, accessToken: "EAAG-stored-token"})
			}

			identities := identitiesOf(t, p, t.Context())

			if !slices.Equal(identities, test.want) {
				t.Errorf("ChannelIdentities() = %+v, want %+v with the number listed once", identities, test.want)
			}
		})
	}
}

func TestChannelIdentitiesLeaveAnEnvNumberWithoutATokenOut(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.envCredentials = credentials{phoneNumberID: "5550009"}
	connected(t, p, t.Context(), credentials{phoneNumberID: "5550001", accessToken: "EAAG-stored-token"})

	identities := identitiesOf(t, p, t.Context())

	if want := []sdk.ChannelIdentity{{Identifier: "5550001"}}; !slices.Equal(identities, want) {
		t.Errorf("ChannelIdentities() = %+v, want the stored number alone", identities)
	}
}

func TestChannelIdentitiesLeaveTheEnvSeedToTheDefaultTenant(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		stored []string
		want   []sdk.ChannelIdentity
	}{
		"holding no number":  {},
		"holding its number": {stored: []string{"5550001"}, want: []sdk.ChannelIdentity{{Identifier: "5550001"}}},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newEnvNumberPlugin(t)
			acme := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
			for _, number := range test.stored {
				connected(t, p, acme, credentials{phoneNumberID: number, accessToken: "EAAG-acme-token"})
			}

			identities := identitiesOf(t, p, acme)

			if !slices.Equal(identities, test.want) {
				t.Errorf("ChannelIdentities() = %+v from another tenant, want the env number withheld", identities)
			}
		})
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

	identitiesOf(t, p, mine)

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

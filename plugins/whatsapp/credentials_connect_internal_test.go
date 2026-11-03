// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

func TestASecondNumberJoinsTheFirst(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	connected(t, p, mine, credentials{phoneNumberID: "5550001", accessToken: "EAAG-first-token"})

	connected(t, p, mine, credentials{phoneNumberID: "5550002", accessToken: "EAAG-second-token"})

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

func TestReconnectingANumberReplacesItsTokenAndKeepsTheRest(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	mine := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	fullyConnected(t, p, mine)
	before, digestBefore := storedSecrets(t, p)
	want := credentials{phoneNumberID: "5550001", accessToken: "EAAG-replaced-token"}

	connected(t, p, mine, want)

	held, err := p.credentialsFor(mine, "5550001")
	if err != nil || held != want {
		t.Errorf("credentialsFor() = %+v, %v, want %+v", held, err, want)
	}
	identities, err := p.ChannelIdentities(mine)
	wantKept := []sdk.ChannelIdentity{{Identifier: "5550001", Label: "Sales"}}
	if err != nil || !slices.Equal(identities, wantKept) {
		t.Errorf("ChannelIdentities() = %+v, %v, want the label kept", identities, err)
	}
	after, digestAfter := storedSecrets(t, p)
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
	appSecret, digest := storedSecrets(t, p)
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
	connected(t, p, sdk.WithTenant(t.Context(), acme),
		credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"})

	err := p.SetChannelCredentials(elsewhere, sdk.ChannelCredentials{
		Identifier: "5550001", Secret: "EAAG-globex-token", AppSecret: "app-secret-globex",
	})

	var refused sdk.GraphError
	if !errors.As(err, &refused) {
		t.Fatalf("SetChannelCredentials() error = %v, want the reason channel_number_taken", err)
	}
	if refused.Code != "CONFLICT" || refused.Reason != "channel_number_taken" {
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
	connected(t, p, mine, credentials{phoneNumberID: "5550009", accessToken: "EAAG-first-token"})
	p.envCredentials = credentials{phoneNumberID: "5550009", accessToken: "EAAG-env-token"}
	want := credentials{phoneNumberID: "5550009", accessToken: "EAAG-replaced-token"}

	connected(t, p, mine, want)

	held, err := p.credentialsFor(mine, "5550009")
	if err != nil || held != want {
		t.Errorf("credentialsFor() = %+v, %v, want %+v", held, err, want)
	}
}

func TestAWorkspaceNotHoldingTheEnvironmentNumberIsTurnedDownBesideAHolder(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	acme := sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
	elsewhere := sdk.WithTenant(t.Context(), seededTenant(t, p, "Globex"))
	connected(t, p, acme, credentials{phoneNumberID: "5550009", accessToken: "EAAG-first-token"})
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
	connected(t, p, mine, credentials{phoneNumberID: "5550001", accessToken: "EAAG-first-token"})
	connected(t, p, mine, credentials{phoneNumberID: "5550002", accessToken: "EAAG-second-token"})

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
	connected(t, p, acme, credentials{phoneNumberID: "5550001", accessToken: "EAAG-acme-token"})

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

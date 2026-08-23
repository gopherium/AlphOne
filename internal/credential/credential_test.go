// SPDX-License-Identifier: Elastic-2.0

package credential_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/credential"
)

func TestOriginNamesTheTokenTheRequestCarries(t *testing.T) {
	t.Parallel()

	ctx := credential.WithToken(t.Context(), credential.Token{Name: "n8n production"})

	if got, want := credential.Origin(ctx), "token:n8n production"; got != want {
		t.Errorf("Origin() = %q, want %q", got, want)
	}
	if got := credential.Origin(t.Context()); got != "" {
		t.Errorf("Origin(bare context) = %q, want empty", got)
	}
}

func TestTokenOfReturnsThePrincipalTheRequestCarries(t *testing.T) {
	t.Parallel()

	id := uuid.Must(uuid.NewV7())
	stamped := credential.Token{ID: id, Name: "n8n production", Scopes: apitoken.ParseScopes("tasks:write")}

	carried, ok := credential.TokenOf(credential.WithToken(t.Context(), stamped))

	if !ok {
		t.Fatal("TokenOf() reported no token, want the stamped one")
	}
	if carried.ID != id {
		t.Errorf("ID = %v, want %v", carried.ID, id)
	}
	if got, want := carried.Scopes.String(), "tasks:write"; got != want {
		t.Errorf("Scopes = %q, want %q", got, want)
	}
}

func TestTokenOfReportsASessionCarriesNoToken(t *testing.T) {
	t.Parallel()

	if _, ok := credential.TokenOf(t.Context()); ok {
		t.Error("TokenOf(bare context) reported a token, want none")
	}
}

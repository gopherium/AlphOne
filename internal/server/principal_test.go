// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/credential"
)

// principalHandler answers the API token principal the request context carries.
func principalHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := credential.TokenOf(r.Context())
		if !ok {
			http.Error(w, "no token principal", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "%s|%s|%s", token.ID, token.Name, token.Scopes)
	})
}

// newPrincipalServer returns a server whose probe route answers the caller's token principal.
func newPrincipalServer(t *testing.T) (http.Handler, *fakeTokenStore, *testkit.Store) {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	tokens := newFakeTokenStore()
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Tasks:    newFakeTaskStore(),
		Users:    users,
		Tokens:   tokens,
		Version:  "9.9.9",
		Plugins:  map[string]http.Handler{"probe": principalHandler()},
	})
	return handler, tokens, users
}

func TestBearerTokenStampsItsPrincipalOnTheContext(t *testing.T) {
	t.Parallel()

	handler, tokens, users := newPrincipalServer(t)
	ada, err := users.UserByEmail(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	granted := apitoken.ParseScopes("contacts:read tasks:write")
	minted, err := apitoken.Mint(ada.ID, "n8n production", granted, apitoken.Never)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	tokens.tokens[minted.Token.Hash] = minted.Token

	recorder := getWithBearer(handler, "/api/plugins/probe/principal", minted.Secret)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	want := fmt.Sprintf("%s|n8n production|contacts:read tasks:write", minted.Token.ID)
	if got := recorder.Body.String(); got != want {
		t.Errorf("principal = %q, want %q", got, want)
	}
}

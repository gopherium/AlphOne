// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"testing"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/apitoken"
)

// mintScoped stores a token for the seeded user granted scopes and returns its secret.
func mintScoped(t *testing.T, tokens *fakeTokenStore, users *testkit.Store, scopes string) string {
	t.Helper()
	ada, err := users.UserByEmail(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	minted, err := apitoken.Mint(ada.ID, "scoped", apitoken.ParseScopes(scopes), apitoken.Never)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	tokens.tokens[minted.Token.Hash] = minted.Token
	return minted.Secret
}

// gateAnswer is the envelope every scope gate step reads.
type gateAnswer struct {
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

func TestReadScopedTokenReachesTheContactListing(t *testing.T) {
	t.Parallel()

	handler, tokens, users, _ := newTokenServer(t)
	secret := mintScoped(t, tokens, users, "contacts:read")

	recorder := postGraphWithBearer(handler,
		`{"query":"{ contacts(first: 1) { edges { node { id } } } }"}`, secret)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	answered := decodeBody[gateAnswer](t, recorder)
	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, the token holds contacts:read", answered.Errors)
	}
}

func TestReadScopedTokenIsRefusedAContactWrite(t *testing.T) {
	t.Parallel()

	handler, tokens, users, _ := newTokenServer(t)
	secret := mintScoped(t, tokens, users, "contacts:read")

	recorder := postGraphWithBearer(handler,
		`{"query":"mutation { createContact(name: \"Maria Perez\") { id } }"}`, secret)

	answered := decodeBody[gateAnswer](t, recorder)
	if len(answered.Errors) != 1 {
		t.Fatalf("errors = %v, want exactly one refusal", answered.Errors)
	}
	if got := answered.Errors[0].Extensions["scope"]; got != "contacts:write" {
		t.Errorf("scope = %v, want contacts:write named by the real schema", got)
	}
	if got := answered.Errors[0].Extensions["code"]; got != "UNAUTHORIZED" {
		t.Errorf("code = %v, want UNAUTHORIZED", got)
	}
}

func TestScopedTokenStaysInsideItsGrantedAreas(t *testing.T) {
	t.Parallel()

	handler, tokens, users, _ := newTokenServer(t)
	secret := mintScoped(t, tokens, users, "tasks:write")

	recorder := postGraphWithBearer(handler, `{"query":"{ webhooks { id } }"}`, secret)

	answered := decodeBody[gateAnswer](t, recorder)
	if len(answered.Errors) != 1 {
		t.Fatalf("errors = %v, want exactly one refusal", answered.Errors)
	}
	if got := answered.Errors[0].Extensions["scope"]; got != "webhooks:read" {
		t.Errorf("scope = %v, want webhooks:read", got)
	}
}

func TestScopedTokenIsRefusedThroughAFragment(t *testing.T) {
	t.Parallel()

	handler, tokens, users, _ := newTokenServer(t)
	secret := mintScoped(t, tokens, users, "tasks:read")

	recorder := postGraphWithBearer(handler,
		`{"query":"{ ...hidden } fragment hidden on Query { webhooks { id } }"}`, secret)

	answered := decodeBody[gateAnswer](t, recorder)
	if len(answered.Errors) != 1 {
		t.Fatalf("errors = %v, want the fragment wrapped field refused", answered.Errors)
	}
	if got := answered.Errors[0].Extensions["scope"]; got != "webhooks:read" {
		t.Errorf("scope = %v, want webhooks:read, a fragment is no escape", got)
	}
}

func TestScopedTokenStillReadsItsOwnIdentity(t *testing.T) {
	t.Parallel()

	handler, tokens, users, _ := newTokenServer(t)
	secret := mintScoped(t, tokens, users, "tasks:read")

	recorder := postGraphWithBearer(handler, `{"query":"{ me { email } }"}`, secret)

	answered := decodeBody[gateAnswer](t, recorder)
	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, the auth area is open to every caller", answered.Errors)
	}
}

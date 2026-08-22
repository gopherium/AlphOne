// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/role"
)

// roleHandler answers the tier the request context carries.
func roleHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, authkit.IdentityFromContext(r.Context()).Role)
	})
}

// roleServer bundles a server answering the caller's tier with the stores behind it.
type roleServer struct {
	handler http.Handler
	users   *testkit.Store
	tokens  *fakeTokenStore
	ada     uuid.UUID
}

// newRoleServer returns a server whose probe route answers the caller's tier.
func newRoleServer(t *testing.T, tier role.Role) roleServer {
	t.Helper()
	users := newFakeUserStore()
	ada := addAda(t, users)
	ada.Role = tier.String()
	users.Users[ada.ID] = ada
	tokens := newFakeTokenStore()
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Tasks:    newFakeTaskStore(),
		Users:    users,
		Tokens:   tokens,
		Version:  "9.9.9",
		Plugins:  map[string]http.Handler{"probe": roleHandler()},
	})
	return roleServer{handler: handler, users: users, tokens: tokens, ada: ada.ID}
}

// mintFor stores a full scope token for the seeded user and returns its secret.
func (s roleServer) mintFor(t *testing.T) string {
	t.Helper()
	minted, err := apitoken.Mint(s.ada, "n8n", apitoken.Full(), apitoken.Never)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	s.tokens.tokens[minted.Token.Hash] = minted.Token
	return minted.Secret
}

// getWithCookie issues a GET carrying the given session cookie.
func getWithCookie(handler http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestBearerTokenCarriesItsOwnersRole(t *testing.T) {
	t.Parallel()

	server := newRoleServer(t, role.Admin)
	secret := server.mintFor(t)

	recorder := getWithBearer(server.handler, "/api/plugins/probe/role", secret)

	if got := recorder.Body.String(); got != role.Admin.String() {
		t.Errorf("role = %q, want %q", got, role.Admin.String())
	}
}

func TestBearerTokenOfAnAccountHoldingNoRoleCarriesNone(t *testing.T) {
	t.Parallel()

	server := newRoleServer(t, "")
	secret := server.mintFor(t)

	recorder := getWithBearer(server.handler, "/api/plugins/probe/role", secret)

	if got := recorder.Body.String(); got != "" {
		t.Errorf("role = %q, want it empty, an account holding none carries none", got)
	}
}

func TestASessionCarriesItsOwnersRole(t *testing.T) {
	t.Parallel()

	server := newRoleServer(t, role.Admin)
	cookie := loginCookie(t, server.handler)

	recorder := getWithCookie(server.handler, "/api/plugins/probe/role", cookie)

	if got := recorder.Body.String(); got != role.Admin.String() {
		t.Errorf("role = %q, want %q", got, role.Admin.String())
	}
}

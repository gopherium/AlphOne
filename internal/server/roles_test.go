// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/credential"
	"github.com/gopherium/alphone/internal/role"
)

// fakeRoleStore answers the tier of the users it was told about.
type fakeRoleStore struct {
	tiers map[uuid.UUID]role.Role
	err   error
}

// RoleOf returns the stored tier, member for a user it holds nothing for.
func (s *fakeRoleStore) RoleOf(_ context.Context, userID uuid.UUID) (role.Role, error) {
	if s.err != nil {
		return role.Member, s.err
	}
	tier, ok := s.tiers[userID]
	if !ok {
		return role.Member, nil
	}
	return tier, nil
}

// roleHandler answers the tier the request context carries.
func roleHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, credential.RoleOf(r.Context()))
	})
}

// roleServer bundles a server answering the caller's tier with the stores behind it.
type roleServer struct {
	handler http.Handler
	users   *testkit.Store
	tokens  *fakeTokenStore
	roles   *fakeRoleStore
	ada     uuid.UUID
}

// newRoleServer returns a server whose probe route answers the caller's tier.
func newRoleServer(t *testing.T, storeErr error) roleServer {
	t.Helper()
	users := newFakeUserStore()
	ada := addAda(t, users)
	tokens := newFakeTokenStore()
	roles := &fakeRoleStore{tiers: map[uuid.UUID]role.Role{}, err: storeErr}
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Tasks:    newFakeTaskStore(),
		Users:    users,
		Tokens:   tokens,
		Roles:    roles,
		Version:  "9.9.9",
		Plugins:  map[string]http.Handler{"probe": roleHandler()},
	})
	return roleServer{handler: handler, users: users, tokens: tokens, roles: roles, ada: ada.ID}
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

	server := newRoleServer(t, nil)
	server.roles.tiers[server.ada] = role.Admin
	secret := server.mintFor(t)

	recorder := getWithBearer(server.handler, "/api/plugins/probe/role", secret)

	if got := recorder.Body.String(); got != role.Admin.String() {
		t.Errorf("role = %q, want %q", got, role.Admin.String())
	}
}

func TestBearerTokenOfAUserWithNoRowCarriesMember(t *testing.T) {
	t.Parallel()

	server := newRoleServer(t, nil)
	secret := server.mintFor(t)

	recorder := getWithBearer(server.handler, "/api/plugins/probe/role", secret)

	if got := recorder.Body.String(); got != role.Member.String() {
		t.Errorf("role = %q, want %q, a user with no row is a member", got, role.Member.String())
	}
}

func TestASessionCarriesItsOwnersRole(t *testing.T) {
	t.Parallel()

	server := newRoleServer(t, nil)
	server.roles.tiers[server.ada] = role.Admin
	cookie := loginCookie(t, server.handler)

	recorder := getWithCookie(server.handler, "/api/plugins/probe/role", cookie)

	if got := recorder.Body.String(); got != role.Admin.String() {
		t.Errorf("role = %q, want %q, a session carries its role like a token does", got, role.Admin.String())
	}
}

func TestAFailingRoleStoreRefusesASessionToo(t *testing.T) {
	t.Parallel()

	server := newRoleServer(t, nil)
	cookie := loginCookie(t, server.handler)
	server.roles.err = errTokenBackend

	recorder := getWithCookie(server.handler, "/api/plugins/probe/role", cookie)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d on the session path", recorder.Code, http.StatusInternalServerError)
	}
}

func TestAFailingRoleStoreLeavesTheGraphCallerAnonymous(t *testing.T) {
	t.Parallel()

	server := newRoleServer(t, nil)
	cookie := loginCookie(t, server.handler)
	server.roles.err = errTokenBackend

	request := httptest.NewRequest(http.MethodPost, "/api/graphql",
		strings.NewReader(`{"query":"{ version }"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	server.handler.ServeHTTP(recorder, request)

	if !strings.Contains(recorder.Body.String(), "UNAUTHENTICATED") {
		t.Errorf("body = %s, want the caller left anonymous rather than trusted", recorder.Body.String())
	}
}

func TestAFailingRoleStoreRefusesRatherThanDemotes(t *testing.T) {
	t.Parallel()

	server := newRoleServer(t, errTokenBackend)
	secret := server.mintFor(t)

	recorder := getWithBearer(server.handler, "/api/plugins/probe/role", secret)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d, an unreadable role refuses rather than demotes",
			recorder.Code, http.StatusInternalServerError)
	}
}

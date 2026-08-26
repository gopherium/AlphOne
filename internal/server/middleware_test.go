// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/tenant"
	"github.com/gopherium/alphone/sdk"
)

// standingTenantStore places named users in a tenant, the rest in the default.
type standingTenantStore struct {
	standing map[uuid.UUID]uuid.UUID
}

// TenantForUser answers the tenant the user stands in.
func (s standingTenantStore) TenantForUser(_ context.Context, userID uuid.UUID) (tenant.Tenant, error) {
	held, ok := s.standing[userID]
	if !ok {
		held = tenant.DefaultID
	}
	return tenant.Tenant{ID: held, Name: "Standing"}, nil
}

// failingTenantStore refuses every lookup.
type failingTenantStore struct{}

// TenantForUser reports the store failure.
func (failingTenantStore) TenantForUser(context.Context, uuid.UUID) (tenant.Tenant, error) {
	return tenant.Tenant{}, errors.New("tenant store down")
}

// echoHandler returns a plugin handler writing a fixed body.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("plugin says hi"))
	})
}

// newProtectedServer returns a server carrying the echo plugin with one public path, and its user store.
func newProtectedServer(t *testing.T) (http.Handler, *testkit.Store) {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Users:    users,
		Plugins: map[string]http.Handler{
			"echo": echoHandler(),
		},
		PluginPublicPaths: map[string][]string{
			"echo": {"/hook"},
		},
	})
	return handler, users
}

func TestMiddlewareHandsThePluginTheActingUser(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	ada := addAda(t, users)
	var seen uuid.UUID
	var known bool
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Users:    users,
		Plugins: map[string]http.Handler{
			"echo": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen, known = sdk.UserFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			}),
		},
	})
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/api/plugins/echo/anything", nil)
	request.AddCookie(cookie)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if !known {
		t.Fatal("plugin saw no acting user, want the session user")
	}
	if seen != ada.ID {
		t.Errorf("acting user = %v, want the session user %v", seen, ada.ID)
	}
}

func TestMiddlewareHandsThePluginTheActingTenant(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	ada := addAda(t, users)
	acme := uuid.Must(uuid.NewV7())
	var seen uuid.UUID
	var known bool
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Users:    users,
		Tenants:  standingTenantStore{standing: map[uuid.UUID]uuid.UUID{ada.ID: acme}},
		Plugins: map[string]http.Handler{
			"echo": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen, known = sdk.TenantFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			}),
		},
	})
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/api/plugins/echo/anything", nil)
	request.AddCookie(cookie)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if !known {
		t.Fatal("plugin saw no tenant, want the session user's tenant")
	}
	if seen != acme {
		t.Errorf("tenant = %v, want the standing tenant %v", seen, acme)
	}
}

func TestMiddlewareHandsThePluginARequestScope(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	var scopeErr error
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Users:    users,
		Plugins: map[string]http.Handler{
			"echo": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, scopeErr = sdk.ScopedValue(r.Context(), "probe", func() int { return 1 })
				w.WriteHeader(http.StatusOK)
			}),
		},
	})
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/api/plugins/echo/anything", nil)
	request.AddCookie(cookie)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if scopeErr != nil {
		t.Errorf("ScopedValue() error = %v, want a request scope on the plugin path", scopeErr)
	}
}

func TestAPublicPathCarriesNoActingUserOrTenant(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	var sawUser, sawTenant bool
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Users:    users,
		Plugins: map[string]http.Handler{
			"echo": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, sawUser = sdk.UserFromContext(r.Context())
				_, sawTenant = sdk.TenantFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			}),
		},
		PluginPublicPaths: map[string][]string{"echo": {"/hook"}},
	})

	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/plugins/echo/hook", nil))

	if sawUser || sawTenant {
		t.Errorf("public path saw user %t tenant %t, want neither installed", sawUser, sawTenant)
	}
}

func TestAPluginRequestReportsAFailingTenantStore(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Users:    users,
		Tenants:  failingTenantStore{},
		Plugins: map[string]http.Handler{
			"echo": echoHandler(),
		},
	})
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/api/plugins/echo/anything", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d for an unresolvable tenant",
			recorder.Code, http.StatusInternalServerError)
	}
}

func TestAGraphRequestReportsAFailingTenantStore(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Users:    users,
		Tenants:  failingTenantStore{},
	})
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodPost, "/api/graphql",
		strings.NewReader(`{"query":"{ version }"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d for an unresolvable tenant",
			recorder.Code, http.StatusInternalServerError)
	}
}

func TestMiddlewareRejectsRequestsWithoutASession(t *testing.T) {
	t.Parallel()

	handler, _ := newProtectedServer(t)

	for _, target := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/plugins/echo/conversations"},
		{http.MethodPost, "/api/plugins/echo/conversations"},
	} {
		request := httptest.NewRequest(target.method, target.path, strings.NewReader("{}"))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want %d", target.method, target.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestMiddlewareAdmitsAuthenticatedRequests(t *testing.T) {
	t.Parallel()

	handler, _ := newProtectedServer(t)
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/api/plugins/echo/anything", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "plugin says hi" {
		t.Errorf("response = %d %q, want the plugin handler's response", recorder.Code, recorder.Body.String())
	}
}

func TestMiddlewareAdmitsDeclaredPublicPluginPaths(t *testing.T) {
	t.Parallel()

	handler, _ := newProtectedServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "/api/plugins/echo/hook", strings.NewReader("{}"))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Errorf("%s public path status = %d, want %d", method, recorder.Code, http.StatusOK)
		}
	}
}

func TestMiddlewareReportsSessionStoreFailure(t *testing.T) {
	t.Parallel()

	handler, users := newProtectedServer(t)
	cookie := loginCookie(t, handler)
	users.SessionErr = context.DeadlineExceeded

	request := httptest.NewRequest(http.MethodGet, "/api/plugins/echo/anything", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/sdk"
)

func echoHandler(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func newProtectedServer(t *testing.T) (http.Handler, *testkit.Store) {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	handler := server.NewServer(server.Config{
		Contacts: newFakeContactStore(),
		Users:    users,
		Plugins: map[string]http.Handler{
			"echo": echoHandler(http.StatusOK, "plugin says hi"),
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
	handler := server.NewServer(server.Config{
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

func TestMiddlewareRejectsRequestsWithoutASession(t *testing.T) {
	t.Parallel()

	handler, _ := newProtectedServer(t)

	for _, target := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/contacts"},
		{http.MethodGet, "/api/contacts/00000000-0000-0000-0000-000000000000"},
		{http.MethodGet, "/api/plugins/echo/conversations"},
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

func TestMiddlewareAdmitsContactWritesWithASession(t *testing.T) {
	t.Parallel()

	handler, _ := newProtectedServer(t)
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodPost, "/api/contacts", strings.NewReader(`{"name":"Grace"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
}

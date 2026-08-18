// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/apitoken"
)

// newAreaServer returns a server whose echo plugin serves the media area.
func newAreaServer(t *testing.T) (http.Handler, *fakeTokenStore, *testkit.Store) {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	tokens := newFakeTokenStore()
	handler := newGraphServer(t, graphConfig{
		Contacts:          newFakeContactStore(),
		Tasks:             newFakeTaskStore(),
		Users:             users,
		Tokens:            tokens,
		Version:           "9.9.9",
		Plugins:           map[string]http.Handler{"echo": echoHandler(http.StatusOK, "plugin says hi")},
		PluginAreas:       map[string]string{"echo": "contacts"},
		PluginPublicPaths: map[string][]string{"echo": {"/webhook"}},
	})
	return handler, tokens, users
}

// postWithBearer issues a POST carrying the given bearer credential.
func postWithBearer(handler http.Handler, path, secret string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestATokenOutsideThePluginsAreaIsRefusedItsRoutes(t *testing.T) {
	t.Parallel()

	handler, tokens, users := newAreaServer(t)
	secret := mintScoped(t, tokens, users, "tasks:read")

	recorder := getWithBearer(handler, "/api/plugins/echo/ping", secret)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d, a plugin route is held to its own area", recorder.Code,
			http.StatusForbidden)
	}
}

func TestATokenHoldingThePluginsAreaReachesItsRoutes(t *testing.T) {
	t.Parallel()

	handler, tokens, users := newAreaServer(t)
	secret := mintScoped(t, tokens, users, "contacts:read")

	recorder := getWithBearer(handler, "/api/plugins/echo/ping", secret)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d for a token holding the area", recorder.Code, http.StatusOK)
	}
}

func TestAReadScopedTokenIsRefusedAPluginWrite(t *testing.T) {
	t.Parallel()

	handler, tokens, users := newAreaServer(t)
	secret := mintScoped(t, tokens, users, "contacts:read")

	recorder := postWithBearer(handler, "/api/plugins/echo/ping", secret)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d, anything but a GET writes", recorder.Code, http.StatusForbidden)
	}
}

func TestAWildcardTokenReachesEveryPluginArea(t *testing.T) {
	t.Parallel()

	handler, tokens, users := newAreaServer(t)
	secret := mintScoped(t, tokens, users, apitoken.Wildcard)

	recorder := getWithBearer(handler, "/api/plugins/echo/ping", secret)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, the wildcard holds every area", recorder.Code, http.StatusOK)
	}
}

func TestAPluginsPublicPathStaysOpenToTheArea(t *testing.T) {
	t.Parallel()

	handler, _, _ := newAreaServer(t)

	request := httptest.NewRequest(http.MethodPost, "/api/plugins/echo/webhook", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, a public path answers whoever calls it", recorder.Code,
			http.StatusOK)
	}
}

func TestAPluginDeclaringNoAreaKeepsItsRoutesOpenToEveryToken(t *testing.T) {
	t.Parallel()

	handler, tokens, users, _ := newTokenServer(t)
	secret := mintScoped(t, tokens, users, "tasks:read")

	recorder := getWithBearer(handler, "/api/plugins/echo/ping", secret)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, a plugin naming no area keeps today's behaviour",
			recorder.Code, http.StatusOK)
	}
}

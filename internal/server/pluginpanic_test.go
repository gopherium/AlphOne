// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/gouncer/authkit/testkit"
)

// panickingRoute returns a plugin handler that panics with boom, first sending part of a 200 answer when began.
func panickingRoute(began bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		if began {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "partial")
		}
		panic("boom")
	})
}

// panickingPluginServer returns a server whose plugin probe panics on every route, its token store, users and log.
func panickingPluginServer(
	t *testing.T, began bool, perUser int,
) (http.Handler, *fakeTokenStore, *testkit.Store, *bytes.Buffer) {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	tokens := newFakeTokenStore()
	logged := &bytes.Buffer{}
	handler := newGraphServer(t, graphConfig{
		Contacts:          newFakeContactStore(),
		Users:             users,
		Tokens:            tokens,
		Logger:            slog.New(slog.NewTextHandler(logged, nil)),
		Plugins:           map[string]http.Handler{"probe": panickingRoute(began)},
		PluginPublicPaths: map[string][]string{"probe": {"/boom"}},
		PluginAreas:       map[string]string{"probe": "contacts"},
		MaxStreamsPerUser: perUser,
	})
	return handler, tokens, users, logged
}

// servePanicking serves r through handler and returns the recorder and what the server got as a panic.
func servePanicking(handler http.Handler, r *http.Request) (recorder *httptest.ResponseRecorder, escaped any) {
	recorder = httptest.NewRecorder()
	defer func() { escaped = recover() }()
	handler.ServeHTTP(recorder, r)
	return recorder, nil
}

// tokenRequest returns a GET of path carrying secret as its bearer credential.
func tokenRequest(path, secret string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	return request
}

// routePanicLine returns the start of the line the server logs for a panic of the route path of probe.
func routePanicLine(path string, answered bool) string {
	return fmt.Sprintf(`msg="plugin route panicked" plugin=probe method=GET path=/api/plugins/probe%s answered=%t `+
		`error="pluginkit: probe route panicked: boom" stack="goroutine `, path, answered)
}

// assertInternalError fails unless no panic escaped and the recorder holds the internal error, kept out of caches.
func assertInternalError(t *testing.T, recorder *httptest.ResponseRecorder, escaped any) {
	t.Helper()
	if escaped != nil || recorder.Code != http.StatusInternalServerError {
		t.Fatalf("answered %d with the panic %v, want 500", recorder.Code, escaped)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil ||
		!maps.Equal(body, map[string]string{"error": "internal error", "code": "internal"}) {
		t.Errorf("the body reads %s (%v), want the internal error", recorder.Body.String(), err)
	}
	if control := recorder.Header().Get("Cache-Control"); control != "no-store" {
		t.Errorf("the answer carries Cache-Control %q, want no-store", control)
	}
}

func TestAPanicInAPluginRouteAnswersTheInternalError(t *testing.T) {
	t.Parallel()

	handler, _, _, logged := panickingPluginServer(t, false, 0)

	recorder, escaped := servePanicking(handler, httptest.NewRequest(http.MethodGet, "/api/plugins/probe/boom", nil))

	assertInternalError(t, recorder, escaped)
	if encoding := recorder.Header().Get("Content-Encoding"); encoding != "" {
		t.Errorf("the answer carries the plugin's Content-Encoding %q, want none", encoding)
	}
	if line := routePanicLine("/boom", true); !strings.Contains(logged.String(), line) {
		t.Errorf("the server logged %q, want %q among it", logged.String(), line)
	}
}

func TestAPanicAfterAPluginRouteBeganItsAnswerCutsIt(t *testing.T) {
	t.Parallel()

	handler, _, _, logged := panickingPluginServer(t, true, 0)

	_, escaped := servePanicking(handler, httptest.NewRequest(http.MethodGet, "/api/plugins/probe/boom", nil))

	if escaped != http.ErrAbortHandler {
		t.Errorf("the server got the panic %v, want http.ErrAbortHandler", escaped)
	}
	if line := routePanicLine("/boom", false); !strings.Contains(logged.String(), line) {
		t.Errorf("the server logged %q, want %q among it", logged.String(), line)
	}
}

func TestAPanicBehindATokenAnswersTheInternalError(t *testing.T) {
	t.Parallel()

	handler, tokens, users, logged := panickingPluginServer(t, false, 0)
	secret := mintScoped(t, tokens, users, "contacts:read")

	recorder, escaped := servePanicking(handler, tokenRequest("/api/plugins/probe/private", secret))

	assertInternalError(t, recorder, escaped)
	if line := routePanicLine("/private", true); !strings.Contains(logged.String(), line) {
		t.Errorf("the server logged %q, want %q among it", logged.String(), line)
	}
}

// assertCut fails unless the server got http.ErrAbortHandler as the panic, so the answer is cut.
func assertCut(t *testing.T, recorder *httptest.ResponseRecorder, escaped any) {
	t.Helper()
	if escaped != http.ErrAbortHandler {
		t.Fatalf("answered %d with the panic %v, want http.ErrAbortHandler", recorder.Code, escaped)
	}
}

func TestAPanickingPluginRouteGivesBackItsStreamSlot(t *testing.T) {
	t.Parallel()

	outcomes := map[string]struct {
		began  bool
		assert func(*testing.T, *httptest.ResponseRecorder, any)
	}{
		"answered": {began: false, assert: assertInternalError},
		"cut":      {began: true, assert: assertCut},
	}
	for name, outcome := range outcomes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler, tokens, users, _ := panickingPluginServer(t, outcome.began, 1)
			secret := mintScoped(t, tokens, users, "contacts:read")

			for range 2 {
				recorder, escaped := servePanicking(handler, tokenRequest("/api/plugins/probe/private", secret))

				outcome.assert(t, recorder, escaped)
			}
		})
	}
}

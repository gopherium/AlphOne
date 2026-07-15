// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gopherium/alphone/internal/server"
)

func spaServer(t *testing.T) http.Handler {
	t.Helper()
	return server.NewServer(server.Config{
		Contacts: newFakeContactStore(),
		Users:    newFakeUserStore(),
		Web: fstest.MapFS{
			"index.html":    {Data: []byte("<!doctype html><title>AlphOne</title>")},
			"assets/app.js": {Data: []byte("console.log('app')")},
		},
	})
}

func TestServesTheSPAAtTheRoot(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), http.MethodGet, "/", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "AlphOne") {
		t.Errorf("body = %q, want the SPA index.html", recorder.Body.String())
	}
}

func TestServesSPAAssets(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), http.MethodGet, "/assets/app.js", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "console.log") {
		t.Errorf("body = %q, want the asset contents", recorder.Body.String())
	}
}

func TestFallsBackToIndexForClientRoutes(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), http.MethodGet, "/users", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "AlphOne") {
		t.Errorf("client route body = %q, want the SPA index.html fallback", recorder.Body.String())
	}
}

func TestUnknownAPIPathIsNotServedTheSPA(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), http.MethodGet, "/api/nope", "")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if strings.Contains(recorder.Body.String(), "AlphOne") {
		t.Error("an unknown API path was served the SPA, want a JSON 404")
	}
}

func TestWithoutWebFSUnknownPathsAre404(t *testing.T) {
	t.Parallel()

	srv := server.NewServer(server.Config{Contacts: newFakeContactStore(), Users: newFakeUserStore()})

	recorder := doRequest(t, srv, http.MethodGet, "/", "")

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d when no SPA is configured", recorder.Code, http.StatusNotFound)
	}
}

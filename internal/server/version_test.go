// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"testing"

	"github.com/gopherium/alphone/internal/server"
)

type versionBody struct {
	Version string `json:"version"`
}

func TestVersionEndpoint(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	srv := server.NewServer(server.Config{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})

	if code := doRequest(t, srv, http.MethodGet, "/api/version", "").Code; code != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET /api/version = %d, want %d", code, http.StatusUnauthorized)
	}

	cookie := loginCookie(t, srv)
	authed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		srv.ServeHTTP(w, r)
	})
	recorder := doRequest(t, authed, http.MethodGet, "/api/version", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if body := decodeBody[versionBody](t, recorder); body.Version != "9.9.9" {
		t.Errorf("version = %q, want %q", body.Version, "9.9.9")
	}
}

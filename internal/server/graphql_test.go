// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/alphone/internal/server"
)

// postGraphQL posts a GraphQL request body to /api/graphql.
func postGraphQL(t *testing.T, handler http.Handler, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// graphqlData is the data envelope of a version query response.
type graphqlData struct {
	Data struct {
		Version string `json:"version"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

const versionQuery = `{"query":"{ version }"}`

func TestGraphQLRequiresAuthentication(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := server.NewServer(server.Config{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})

	recorder := postGraphQL(t, srv, versionQuery, nil)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated POST /api/graphql = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestGraphQLVersionQueryWithSessionCookie(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := server.NewServer(server.Config{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
	cookie := loginCookie(t, srv)

	recorder := postGraphQL(t, srv, versionQuery, cookie)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := decodeBody[graphqlData](t, recorder)
	if body.Data.Version != "9.9.9" {
		t.Errorf("version = %q, want %q", body.Data.Version, "9.9.9")
	}
	if len(body.Errors) != 0 {
		t.Errorf("errors = %v, want none", body.Errors)
	}
}

func TestGraphQLVersionQueryWithBearerToken(t *testing.T) {
	t.Parallel()

	handler, _, _, secret := newTokenServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(versionQuery))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+secret)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if body := decodeBody[graphqlData](t, recorder); body.Data.Version != "9.9.9" {
		t.Errorf("version = %q, want %q", body.Data.Version, "9.9.9")
	}
}

func TestGraphQLRejectsAnUnknownField(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := server.NewServer(server.Config{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
	cookie := loginCookie(t, srv)

	recorder := postGraphQL(t, srv, `{"query":"{ nope }"}`, cookie)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d: %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
	if body := decodeBody[graphqlData](t, recorder); len(body.Errors) == 0 {
		t.Error("errors are empty, want a validation error")
	}
}

func TestGraphQLRejectsAnOversizedJSONBody(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := server.NewServer(server.Config{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
	cookie := loginCookie(t, srv)
	oversized := `{"query":"{ version }","variables":{"pad":"` + strings.Repeat("x", 1<<20) + `"}}`

	recorder := postGraphQL(t, srv, oversized, cookie)

	body := decodeBody[graphqlData](t, recorder)
	if len(body.Errors) == 0 || !strings.Contains(body.Errors[0].Message, "request body too large") {
		t.Fatalf("errors = %+v, want the body cap rejection", body.Errors)
	}
	if body.Data.Version != "" {
		t.Error("version resolved, want no execution on an oversized body")
	}
}

func TestGraphiQLServesOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	enabled := server.NewServer(server.Config{
		Contacts: newFakeContactStore(), Users: users, Version: "9.9.9", GraphiQL: true,
	})
	cookie := loginCookie(t, enabled)

	request := httptest.NewRequest(http.MethodGet, "/api/graphql", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	enabled.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("enabled GET /api/graphql = %d, want %d", recorder.Code, http.StatusOK)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Errorf("content type = %q, want text/html", contentType)
	}

	disabled := server.NewServer(server.Config{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
	request = httptest.NewRequest(http.MethodGet, "/api/graphql", nil)
	request.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	disabled.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("disabled GET /api/graphql = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

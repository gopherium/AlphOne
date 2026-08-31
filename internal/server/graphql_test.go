// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/ratelimit"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/sdk"
)

// graphConfig carries the stores and bounds a test graph server composes.
type graphConfig struct {
	Contacts          graphres.ContactStore
	Tasks             graphres.TaskStore
	Users             server.UserStore
	Tenants           server.TenantStore
	Tokens            server.TokenStore
	Version           string
	Plugins           map[string]http.Handler
	PluginPublicPaths map[string][]string
	PluginAreas       map[string]string
	FieldSources      []sdk.FieldSource
	MaxStreamLifetime time.Duration
	MaxStreamsPerUser int
	GraphiQL          bool
}

// newGraphServer returns a server whose graph root composes cfg's stores with
// graph plugins whose lazy pools never connect.
func newGraphServer(t *testing.T, cfg graphConfig) http.Handler {
	t.Helper()
	return newSubscribingGraphServer(t, cfg, nil)
}

// newSubscribingGraphServer returns a graph server whose subscriptions read hub.
func newSubscribingGraphServer(t *testing.T, cfg graphConfig, hub *event.Hub) http.Handler {
	t.Helper()
	auth := authkit.New(authkit.Config{
		Store:      cfg.Users,
		CookieName: server.SessionCookieName,
		Privileged: role.Privileged(),
	})
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: cfg.Users, Privileged: role.Privileged()})
	plugins, err := graphroot.All(sdk.Deps{DatabaseURL: "postgres://graph:graph@localhost:1/graph"})
	if err != nil {
		t.Fatalf("graphroot.All() error = %v, want nil", err)
	}
	for _, plugin := range plugins {
		t.Cleanup(func() { _ = plugin.Stop(context.Background()) })
	}
	resolver := &graphres.Resolver{
		Version:      cfg.Version,
		Contacts:     cfg.Contacts,
		Tasks:        cfg.Tasks,
		Auth:         auth,
		Admin:        admin,
		LoginLimiter: ratelimit.NewLimiter(ratelimit.Config{}),
		TokenLimiter: ratelimit.NewLimiter(ratelimit.Config{}),
	}
	if store, ok := cfg.Users.(authkit.InviteStore); ok {
		resolver.Invites = authkit.NewInvites(authkit.InvitesConfig{Store: store})
		resolver.Accounts = store
	}
	if hub != nil {
		resolver.Live = hub
	}
	root, err := graphroot.FromPlugins(resolver, plugins)
	if err != nil {
		t.Fatalf("graphroot.FromPlugins() error = %v, want nil", err)
	}
	return server.NewServer(server.Config{
		Users:             cfg.Users,
		Tenants:           cfg.Tenants,
		Auth:              auth,
		GraphRoot:         root,
		Tokens:            cfg.Tokens,
		Plugins:           cfg.Plugins,
		PluginPublicPaths: cfg.PluginPublicPaths,
		PluginAreas:       cfg.PluginAreas,
		FieldSources:      cfg.FieldSources,
		MaxStreamLifetime: cfg.MaxStreamLifetime,
		MaxStreamsPerUser: cfg.MaxStreamsPerUser,
		GraphiQL:          cfg.GraphiQL,
	})
}

// stubFieldSource serves one fixed field snapshot.
type stubFieldSource struct {
	fields []sdk.GraphField
}

// FieldsSnapshot reports the fixed snapshot.
func (s stubFieldSource) FieldsSnapshot(context.Context) (uint64, []sdk.GraphField, error) {
	return 1, s.fields, nil
}

func TestGraphQLIgnoresAFieldOnACarrierlessType(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	source := stubFieldSource{fields: []sdk.GraphField{
		{Entity: "Task", Name: "estimatedHours", Type: "Int"},
	}}
	srv := newGraphServer(t, graphConfig{
		Contacts:     newFakeContactStore(),
		Users:        users,
		Version:      "9.9.9",
		FieldSources: []sdk.FieldSource{source},
	})
	cookie := loginCookie(t, srv)

	answered := postGraphQL(t, srv, versionQuery, cookie)
	body := decodeBody[graphqlData](t, answered)
	if body.Data.Version != "9.9.9" {
		t.Errorf("version = %q, want the graph unchanged under a carrierless type", body.Data.Version)
	}

	refused := postGraphQL(t, srv,
		`{"query":"{ tasks(date: \"2026-08-12\", first: 1) { edges { node { estimatedHours } } } }"}`, cookie)
	if !strings.Contains(refused.Body.String(), "Cannot query field") {
		t.Errorf("body = %q, want the field refused on a type carrying no carrier", refused.Body.String())
	}
}

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

// urqlAccept is what the graph client sends on an ordinary query, offering
// every answer type it can read including an event stream.
const urqlAccept = "application/graphql-response+json, application/graphql+json, " +
	"application/json, text/event-stream, multipart/mixed"

func TestGraphQLAnswersACallerTakingJSONWithJSON(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := newGraphServer(t, graphConfig{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
	cookie := loginCookie(t, srv)

	request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(versionQuery))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", urqlAccept)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "json") {
		t.Fatalf("content type = %q, want JSON so a query does not spend the stream budget", got)
	}
	body := decodeBody[graphqlData](t, recorder)
	if body.Data.Version != "9.9.9" {
		t.Errorf("version = %q, want %q", body.Data.Version, "9.9.9")
	}
}

func TestGraphQLRequiresAuthentication(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := newGraphServer(t, graphConfig{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})

	recorder := postGraphQL(t, srv, versionQuery, nil)

	body := decodeBody[graphqlData](t, recorder)
	if len(body.Errors) == 0 || body.Data.Version != "" {
		t.Fatalf("anonymous version query = %s, want the gate's rejection", recorder.Body.String())
	}
}

func TestGraphQLVersionQueryWithSessionCookie(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := newGraphServer(t, graphConfig{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
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

func TestGraphQLRejectsAnUnknownBearerToken(t *testing.T) {
	t.Parallel()

	handler, _, _, _ := newTokenServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(versionQuery))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer wrong-secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d: %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
}

func TestGraphQLMasksBearerStoreFailures(t *testing.T) {
	t.Parallel()

	handler, tokens, _, secret := newTokenServer(t)
	tokens.err = errors.New("token store down")
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(versionQuery))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+secret)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d: %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
}

func TestGraphQLRejectsAnUnknownField(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := newGraphServer(t, graphConfig{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
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
	srv := newGraphServer(t, graphConfig{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
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

func TestGraphStreamPassesUnderTheDefaultStreamBounds(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	handler := newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(), Users: users, Version: "9.9.9",
	})
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(versionQuery))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("graph stream without configured bounds = %d, want %d: %s",
			recorder.Code, http.StatusOK, recorder.Body)
	}
}

func TestGraphiQLServesOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	enabled := newGraphServer(t, graphConfig{
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

	disabled := newGraphServer(t, graphConfig{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
	request = httptest.NewRequest(http.MethodGet, "/api/graphql", nil)
	request.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	disabled.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("disabled GET /api/graphql = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

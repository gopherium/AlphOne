// SPDX-License-Identifier: Elastic-2.0

// Package server exposes the CRM core over a JSON HTTP API.
package server

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/ratelimit"
	"github.com/gopherium/pluginkit"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/sdk"
)

// sessionCookieName scopes the login cookie to this product.
const sessionCookieName = "__Host-alphone_session"

// Config carries the stores and plugin surfaces the server serves.
type Config struct {
	Contacts ContactStore
	Tasks    TaskStore
	Users    UserStore
	// Tokens resolves API tokens presented as bearer credentials. Nil
	// leaves the session cookie as the only accepted credential.
	Tokens TokenStore
	// Webhooks persists the outbound event subscriptions the API manages.
	Webhooks WebhookStore
	// Events announces domain events. Nil publishes nothing.
	Events Publisher
	// Live fans published event names out to browser streams. Nil answers
	// the stream route with 503.
	Live *event.Hub
	// Plugins maps a plugin id to its HTTP handler, mounted under
	// /api/plugins/{id}/ behind the session middleware.
	Plugins map[string]http.Handler
	// PluginPublicPaths maps a plugin id to the namespace-relative paths
	// that stay reachable without a session, such as signed webhooks.
	PluginPublicPaths map[string][]string
	// Web serves the single-page app for non-API paths. Nil leaves those
	// paths unhandled, which suits development behind the Vite dev server.
	Web fs.FS
	// TrustedProxies lists the CIDR ranges of reverse proxies permitted to
	// set X-Forwarded-For for the login rate limiter.
	TrustedProxies []string
	// MaxStreamLifetime bounds how long any authenticated plugin request,
	// including an SSE stream, may stay open. Zero applies the host default.
	MaxStreamLifetime time.Duration
	// MaxStreamsPerUser caps concurrent authenticated plugin requests per
	// user. Zero applies the host default.
	MaxStreamsPerUser int
	// Version is the application version reported at /api/version.
	Version string
	// GraphiQL enables the interactive query page on GET /api/graphql.
	GraphiQL bool
}

// NewServer returns the HTTP handler serving the CRM API. Every route
// requires a login session except login, logout, and each plugin's
// declared public paths.
func NewServer(cfg Config) http.Handler {
	maxStreamLifetime, maxStreamsPerUser := streamDefaults(cfg)
	auth := authkit.New(authkit.Config{Store: cfg.Users, CookieName: sessionCookieName})
	admin := authkit.NewAdmin(cfg.Users)
	s := &server{
		store:             cfg.Contacts,
		tasks:             cfg.Tasks,
		auth:              auth,
		users:             cfg.Users,
		tokens:            cfg.Tokens,
		webhooks:          cfg.Webhooks,
		events:            cfg.Events,
		live:              cfg.Live,
		version:           cfg.Version,
		maxStreamLifetime: maxStreamLifetime,
		streams:           newStreamLimiter(maxStreamsPerUser),
	}
	router := chi.NewRouter()
	router.With(ratelimit.Middleware(ratelimit.Config{TrustedProxies: cfg.TrustedProxies})).
		Post("/api/auth/login", auth.Login)
	router.Post("/api/auth/logout", auth.Logout)
	router.Group(func(protected chi.Router) {
		protected.Use(s.requireIdentity)
		protected.Get("/api/auth/session", auth.Session)
		protected.Get("/api/contacts", s.handleContactList())
		protected.Post("/api/contacts", s.handleContactCreate())
		protected.Get("/api/contacts/{id}", s.handleContactGet())
		protected.Patch("/api/contacts/{id}", s.handleContactRename())
		protected.Post("/api/contacts/{id}/identities", s.handleIdentityCreate())
		protected.Delete("/api/contacts/{id}/identities/{identityID}", s.handleIdentityDelete())
		protected.Get("/api/tasks", s.handleTaskList())
		protected.Post("/api/tasks", s.handleTaskCreate())
		protected.Get("/api/tasks/{id}", s.handleTaskGet())
		protected.Patch("/api/tasks/{id}", s.handleTaskPatch())
		protected.Get("/api/users", admin.List)
		protected.Post("/api/users", admin.Create)
		protected.Patch("/api/users/{id}", admin.SetDisabled)
		protected.Method(http.MethodGet, "/api/events", s.boundPluginRequest(s.handleEventStream()))
		protected.Get("/api/webhooks", s.handleWebhookList())
		protected.Post("/api/webhooks", s.handleWebhookCreate())
		protected.Delete("/api/webhooks/{id}", s.handleWebhookDelete())
		protected.Get("/api/version", s.handleVersion())
		protected.Method(http.MethodPost, "/api/graphql", newGraphQLHandler(cfg.Version))
		if cfg.GraphiQL {
			protected.Method(http.MethodGet, "/api/graphql", playground.Handler("AlphOne GraphiQL", "/api/graphql"))
		}
	})
	for id, handler := range cfg.Plugins {
		prefix := "/api/plugins/" + id
		guarded := s.protectPlugin(handler, cfg.PluginPublicPaths[id])
		router.Mount(prefix, http.StripPrefix(prefix, guarded))
	}
	if cfg.Web != nil {
		router.NotFound(spaHandler(cfg.Web))
	}
	return router
}

type server struct {
	store             ContactStore
	tasks             TaskStore
	auth              *authkit.Handlers
	users             UserStore
	tokens            TokenStore
	webhooks          WebhookStore
	events            Publisher
	live              *event.Hub
	version           string
	maxStreamLifetime time.Duration
	streams           *streamLimiter
}

// protectPlugin wraps a plugin handler in the session middleware, letting
// the plugin's declared public paths through untouched.
func (s *server) protectPlugin(handler http.Handler, publicPaths []string) http.Handler {
	return pluginkit.Protect(handler, publicPaths, func(next http.Handler) http.Handler {
		return s.requireIdentity(s.boundPluginRequest(withActingUser(next)))
	})
}

// withActingUser passes the authenticated user to the plugin through the SDK.
func withActingUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := sdk.WithUser(r.Context(), authkit.IdentityFromContext(r.Context()).ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

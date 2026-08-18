// SPDX-License-Identifier: Elastic-2.0

// Package server exposes the CRM core over its GraphQL endpoint and mounts
// the plugin routes.
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

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/internal/mcp"
	"github.com/gopherium/alphone/sdk"
)

// SessionCookieName scopes the login cookie to this product.
const SessionCookieName = "__Host-alphone_session"

// Config carries the stores and plugin surfaces the server serves.
type Config struct {
	Users UserStore
	// Auth validates the login sessions plugin requests present. Nil builds
	// a default over Users with the product cookie.
	Auth *authkit.Handlers
	// GraphRoot is the composed resolver root served at /api/graphql. Nil
	// leaves the graph endpoint unmounted.
	GraphRoot graph.ResolverRoot
	// Tokens resolves API tokens presented as bearer credentials. Nil
	// leaves the session cookie as the only accepted credential.
	Tokens TokenStore
	// Roles reads the tier every caller stands in. Nil leaves every
	// caller a member.
	Roles RoleStore
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
	// set X-Forwarded-For for the graph rate limiter.
	TrustedProxies []string
	// MaxStreamLifetime bounds how long an authenticated plugin request or a
	// graph subscription may stay open. Zero applies the host default.
	MaxStreamLifetime time.Duration
	// MaxStreamsPerUser caps concurrent authenticated plugin requests and
	// graph subscriptions per user. Zero applies the host default.
	MaxStreamsPerUser int
	// FieldSources lists the plugins serving runtime defined graph fields.
	FieldSources []sdk.FieldSource
	// GraphiQL enables the interactive query page on GET /api/graphql.
	GraphiQL bool
	// Version names this build to a connecting agent.
	Version string
}

// NewServer returns the HTTP handler serving the CRM API. Every route
// requires a login session except the graph login mutation and each
// plugin's declared public paths.
func NewServer(cfg Config) http.Handler {
	maxStreamLifetime, maxStreamsPerUser := streamDefaults(cfg)
	auth := cfg.Auth
	if auth == nil {
		auth = authkit.New(authkit.Config{Store: cfg.Users, CookieName: SessionCookieName})
	}
	s := &server{
		auth:              auth,
		users:             cfg.Users,
		tokens:            cfg.Tokens,
		roles:             cfg.Roles,
		maxStreamLifetime: maxStreamLifetime,
		streams:           newStreamLimiter(maxStreamsPerUser),
	}
	router := chi.NewRouter()
	if cfg.GraphRoot != nil {
		graph := newGraphQLHandler(cfg.GraphRoot, maxStreamLifetime, maxStreamsPerUser, cfg.FieldSources)
		router.Group(func(graphed chi.Router) {
			graphed.Use(ratelimit.ResolveClientIP(cfg.TrustedProxies))
			graphed.Use(s.identifyIdentity)
			graphed.Method(http.MethodPost, "/api/graphql", graph)
			if cfg.GraphiQL {
				graphed.Method(http.MethodGet, "/api/graphql", playground.Handler("AlphOne GraphiQL", "/api/graphql"))
			}
		})
		router.Method(http.MethodPost, "/api/mcp", s.requireIdentity(mcp.Handler(graph, cfg.Version)))
	}
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
	auth              *authkit.Handlers
	users             UserStore
	tokens            TokenStore
	roles             RoleStore
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

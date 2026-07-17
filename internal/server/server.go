// SPDX-License-Identifier: Elastic-2.0

// Package server exposes the CRM core over a JSON HTTP API.
package server

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/ratelimit"
)

// sessionCookieName scopes the login cookie to this product.
const sessionCookieName = "__Host-alphone_session"

// Config carries the stores and plugin surfaces the server serves.
type Config struct {
	Contacts ContactStore
	Users    authkit.AdminStore
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
		auth:              auth,
		version:           cfg.Version,
		maxStreamLifetime: maxStreamLifetime,
		streams:           newStreamLimiter(maxStreamsPerUser),
	}
	router := chi.NewRouter()
	router.With(ratelimit.Middleware(ratelimit.Config{TrustedProxies: cfg.TrustedProxies})).
		Post("/api/auth/login", auth.Login)
	router.Post("/api/auth/logout", auth.Logout)
	router.Group(func(protected chi.Router) {
		protected.Use(auth.RequireSession)
		protected.Get("/api/auth/session", auth.Session)
		protected.Post("/api/contacts", s.handleContactCreate())
		protected.Get("/api/contacts/{id}", s.handleContactGet())
		protected.Get("/api/users", admin.List)
		protected.Post("/api/users", admin.Create)
		protected.Patch("/api/users/{id}", admin.SetDisabled)
		protected.Get("/api/version", s.handleVersion())
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
	auth              *authkit.Handlers
	version           string
	maxStreamLifetime time.Duration
	streams           *streamLimiter
}

// protectPlugin wraps a plugin handler in the session middleware, letting
// the plugin's declared public paths through untouched.
func (s *server) protectPlugin(handler http.Handler, publicPaths []string) http.Handler {
	public := make(map[string]struct{}, len(publicPaths))
	for _, path := range publicPaths {
		public[path] = struct{}{}
	}
	protected := s.auth.RequireSession(s.boundPluginRequest(handler))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := public[r.URL.Path]; ok {
			handler.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

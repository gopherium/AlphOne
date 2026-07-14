// SPDX-License-Identifier: Elastic-2.0

// Package server exposes the CRM core over a JSON HTTP API.
package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// UserStore persists users for both login and administration.
type UserStore interface {
	gouncer.Store
	ListUsers(ctx context.Context) ([]gouncer.User, error)
	SetUserDisabled(ctx context.Context, id uuid.UUID, disabled bool) error
}

// Config carries the stores and plugin surfaces the server serves.
type Config struct {
	Contacts ContactStore
	Users    UserStore
	// Plugins maps a plugin id to its HTTP handler, mounted under
	// /api/plugins/{id}/ behind the session middleware.
	Plugins map[string]http.Handler
	// PluginPublicPaths maps a plugin id to the namespace-relative paths
	// that stay reachable without a session, such as signed webhooks.
	PluginPublicPaths map[string][]string
}

// NewServer returns the HTTP handler serving the CRM API. Every route
// requires a login session except the auth endpoints themselves and each
// plugin's declared public paths.
func NewServer(cfg Config) http.Handler {
	s := &server{store: cfg.Contacts, users: cfg.Users, newSession: gouncer.NewSession}
	router := chi.NewRouter()
	router.Post("/api/auth/login", s.handleLogin())
	router.Post("/api/auth/logout", s.handleLogout())
	router.Get("/api/auth/session", s.handleSession())
	router.Group(func(protected chi.Router) {
		protected.Use(s.requireSession)
		protected.Post("/api/contacts", s.handleContactCreate())
		protected.Get("/api/contacts/{id}", s.handleContactGet())
		protected.Get("/api/users", s.handleUserList())
		protected.Post("/api/users", s.handleUserCreate())
		protected.Patch("/api/users/{id}", s.handleUserSetDisabled())
	})
	for id, handler := range cfg.Plugins {
		prefix := "/api/plugins/" + id
		guarded := s.protectPlugin(handler, cfg.PluginPublicPaths[id])
		router.Mount(prefix, http.StripPrefix(prefix, guarded))
	}
	return router
}

type server struct {
	store ContactStore
	users UserStore
	// newSession issues login sessions; a field so failure paths stay
	// testable.
	newSession func(userID uuid.UUID) (gouncer.Session, error)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

// Package server exposes the CRM core over a JSON HTTP API.
package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewServer returns the HTTP handler serving the CRM API backed by
// store, with every plugin handler mounted under /api/plugins/{id}/.
func NewServer(store ContactStore, pluginRoutes map[string]http.Handler) http.Handler {
	s := &server{store: store}
	router := chi.NewRouter()
	router.Post("/api/contacts", s.handleContactCreate())
	router.Get("/api/contacts/{id}", s.handleContactGet())
	for id, handler := range pluginRoutes {
		prefix := "/api/plugins/" + id
		router.Mount(prefix, http.StripPrefix(prefix, handler))
	}
	return router
}

type server struct {
	store ContactStore
}

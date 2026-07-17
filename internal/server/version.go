// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"net/http"

	"github.com/gopherium/gouncer/authkit"
)

type versionResponse struct {
	Version string `json:"version"`
}

// handleVersion returns an http.HandlerFunc reporting the server version.
func (s *server) handleVersion() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		authkit.Respond(w, http.StatusOK, versionResponse{Version: s.version})
	}
}

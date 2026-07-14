// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the single-page app from webFS, falling back to
// index.html for paths without a matching file.
func spaHandler(webFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServerFS(webFS)
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			respondError(w, http.StatusNotFound, "not found")
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" {
			if _, err := fs.Stat(webFS, name); err != nil {
				r = r.Clone(r.Context())
				r.URL.Path = "/"
			}
		}
		fileServer.ServeHTTP(w, r)
	}
}

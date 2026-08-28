// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/gopherium/gouncer/authkit"
)

// assetPrefix names the directory holding the build's content hashed files.
const assetPrefix = "assets/"

// indexCache asks a client to revalidate the page naming the current build.
const indexCache = "no-cache"

// assetCache lets a client keep a content hashed file for a year.
const assetCache = "public, max-age=31536000, immutable"

// hashedAsset matches a built file whose name carries the content hash the bundler appends.
var hashedAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{8}\.[A-Za-z0-9]+$`)

// spaHandler serves the single-page app from webFS, falling back to
// index.html for paths without a matching file.
func spaHandler(webFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServerFS(webFS)
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			authkit.RespondError(w, http.StatusNotFound, authkit.ErrorResponse{Message: "not found"})
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" {
			if _, err := fs.Stat(webFS, name); err != nil {
				r = r.Clone(r.Context())
				r.URL.Path = "/"
				name = ""
			}
		}
		w.Header().Set("Cache-Control", cacheFor(name))
		fileServer.ServeHTTP(w, r)
	}
}

// cacheFor answers how long a client may keep the file a served name resolves to.
func cacheFor(name string) string {
	if strings.HasPrefix(name, assetPrefix) && hashedAsset.MatchString(name) {
		return assetCache
	}
	return indexCache
}

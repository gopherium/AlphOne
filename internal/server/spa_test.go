// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gopherium/alphone/internal/server"
)

// spaServer returns a server over an in memory web filesystem holding an index page and one asset.
func spaServer(t *testing.T) http.Handler {
	t.Helper()
	return server.NewServer(server.Config{
		Users: newFakeUserStore(),
		Web: fstest.MapFS{
			"index.html":               {Data: []byte("<!doctype html><title>AlphOne</title>")},
			"assets/app.js":            {Data: []byte("console.log('app')")},
			"assets/app-B7EuLSNJ.js":   {Data: []byte("console.log('hashed')")},
			"assets/index-Do-XTcIQ.js": {Data: []byte("console.log('hyphenated hash')")},
		},
	})
}

func TestServesTheSPAAtTheRoot(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), "/")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "AlphOne") {
		t.Errorf("body = %q, want the SPA index.html", recorder.Body.String())
	}
}

func TestServesSPAAssets(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), "/assets/app.js")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "console.log") {
		t.Errorf("body = %q, want the asset contents", recorder.Body.String())
	}
}

func TestTheIndexIsRevalidatedOnEveryVisit(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), "/")

	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache so a new build is never hidden behind a cached page", got)
	}
}

func TestAClientRouteIsRevalidatedLikeTheIndex(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), "/users")

	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache because the fallback answers the index", got)
	}
}

func TestHashedAssetsAreKeptForAYear(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), "/assets/app-B7EuLSNJ.js")

	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the year an immutable hashed asset earns", got)
	}
}

func TestAHashHoldingAHyphenIsStillRecognised(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), "/assets/index-Do-XTcIQ.js")

	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the year, the build writes hashes holding a hyphen", got)
	}
}

func TestAnAssetWithoutAHashIsRevalidated(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), "/assets/app.js")

	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache, a stable name can be replaced at the same address", got)
	}
}

func TestFallsBackToIndexForClientRoutes(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), "/users")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "AlphOne") {
		t.Errorf("client route body = %q, want the SPA index.html fallback", recorder.Body.String())
	}
}

func TestUnknownAPIPathIsNotServedTheSPA(t *testing.T) {
	t.Parallel()

	recorder := doRequest(t, spaServer(t), "/api/nope")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if strings.Contains(recorder.Body.String(), "AlphOne") {
		t.Error("an unknown API path was served the SPA, want a JSON 404")
	}
}

func TestWithoutWebFSUnknownPathsAre404(t *testing.T) {
	t.Parallel()

	srv := server.NewServer(server.Config{Users: newFakeUserStore()})

	recorder := doRequest(t, srv, "/")

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d when no SPA is configured", recorder.Code, http.StatusNotFound)
	}
}

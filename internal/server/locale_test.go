// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/gouncer/authkit/testkit"
)

// askLocale posts the locale query carrying the given Accept-Language header.
func askLocale(t *testing.T, handler http.Handler, acceptLanguage string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(`{"query":"{ locale }"}`))
	request.Header.Set("Content-Type", "application/json")
	if acceptLanguage != "" {
		request.Header.Set("Accept-Language", acceptLanguage)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var body struct {
		Data struct {
			Locale string `json:"locale"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the locale answer: %v from %s", err, recorder.Body.String())
	}
	return body.Data.Locale
}

func TestLocaleReadsTheAcceptLanguageHeaderThroughTheServer(t *testing.T) {
	t.Parallel()

	srv := newGraphServer(t, graphConfig{Users: testkit.NewStore()})

	if got := askLocale(t, srv, "es"); got != "es-ES" {
		t.Errorf("locale = %q, want the header matched through the real handler", got)
	}
	if got := askLocale(t, srv, ""); got != "en-US" {
		t.Errorf("locale = %q, want the default without a header", got)
	}
}

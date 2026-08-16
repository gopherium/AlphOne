// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/apitoken"
)

// expire ends the stored token's life an hour ago.
func expire(t *testing.T, tokens *fakeTokenStore, secret string) {
	t.Helper()
	hash := apitoken.HashSecret(secret)
	stored, ok := tokens.tokens[hash]
	if !ok {
		t.Fatalf("no token stored under the minted secret")
	}
	stored.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	tokens.tokens[hash] = stored
}

func TestExpiredBearerTokenIsRejectedOnTheGraph(t *testing.T) {
	t.Parallel()

	handler, tokens, _, secret := newTokenServer(t)
	expire(t, tokens, secret)

	recorder := postGraphWithBearer(handler, `{"query":"{ me { email } }"}`, secret)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d: %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
}

func TestExpiredBearerTokenIsRejectedOnAPluginRoute(t *testing.T) {
	t.Parallel()

	handler, tokens, _, secret := newTokenServer(t)
	expire(t, tokens, secret)

	recorder := getWithBearer(handler, "/api/plugins/echo/conversations", secret)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d, the plugin routes share one gate", recorder.Code, http.StatusUnauthorized)
	}
}

func TestExpiredBearerTokenIsNeverRecordedAsUsed(t *testing.T) {
	t.Parallel()

	handler, tokens, _, secret := newTokenServer(t)
	expire(t, tokens, secret)

	getWithBearer(handler, "/api/plugins/echo/conversations", secret)

	if len(tokens.touched) != 0 {
		t.Errorf("touched = %v, want none, a dead credential never acts", tokens.touched)
	}
}

func TestUnexpiredBearerTokenStillPasses(t *testing.T) {
	t.Parallel()

	handler, tokens, _, secret := newTokenServer(t)
	hash := apitoken.HashSecret(secret)
	stored := tokens.tokens[hash]
	stored.ExpiresAt = time.Now().UTC().Add(time.Hour)
	tokens.tokens[hash] = stored

	recorder := getWithBearer(handler, "/api/plugins/echo/conversations", secret)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

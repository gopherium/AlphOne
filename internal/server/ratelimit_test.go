// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/alphone/internal/server"
)

func loginVia(t *testing.T, handler http.Handler, remoteAddr, forwardedFor, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func loginFrom(t *testing.T, handler http.Handler, ip, body string) *httptest.ResponseRecorder {
	t.Helper()
	return loginVia(t, handler, ip, "", body)
}

func newProxiedAuthServer(users server.UserStore, trustedProxies ...string) http.Handler {
	return server.NewServer(server.Config{
		Contacts:       newFakeContactStore(),
		Users:          users,
		TrustedProxies: trustedProxies,
	})
}

func TestLoginRateLimitBlocksRepeatedAttempts(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newAuthServer(users)
	const wrong = `{"email":"ada@example.com","password":"wrong password!"}`

	if code := loginFrom(t, handler, "198.51.100.7:40000", wrong).Code; code == http.StatusTooManyRequests {
		t.Fatal("the first login attempt was rate limited")
	}

	var last *httptest.ResponseRecorder
	for i := 0; i < 50; i++ {
		last = loginFrom(t, handler, "198.51.100.7:40000", wrong)
		if last.Code == http.StatusTooManyRequests {
			break
		}
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("repeated attempts from one IP were never rate limited, last status = %d", last.Code)
	}
	if body := decodeBody[errorBody](t, last); body.Error == "" {
		t.Error("rate-limit response carries no JSON error message")
	}
	if got := last.Header().Get("Retry-After"); got != "120" {
		t.Errorf("Retry-After = %q, want %q", got, "120")
	}
	for _, header := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"} {
		if value := last.Header().Get(header); value != "" {
			t.Errorf("rate-limit response leaks %s = %q", header, value)
		}
	}
}

func TestLoginRateLimitIsPerIP(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newAuthServer(users)
	const wrong = `{"email":"ada@example.com","password":"wrong password!"}`

	for i := 0; i < 50; i++ {
		loginFrom(t, handler, "198.51.100.8:40000", wrong)
	}

	fresh := loginFrom(t, handler, "203.0.113.9:40000",
		`{"email":"ada@example.com","password":"correct horse battery"}`)

	if fresh.Code != http.StatusOK {
		t.Fatalf("a login from an untouched IP got status %d, want %d", fresh.Code, http.StatusOK)
	}
}

func TestLoginRateLimitDoesNotCountSuccessfulLogins(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newAuthServer(users)
	const good = `{"email":"ada@example.com","password":"correct horse battery"}`

	for i := range 30 {
		if code := loginFrom(t, handler, "198.51.100.40:5000", good).Code; code != http.StatusOK {
			t.Fatalf("successful login %d got status %d, want %d", i+1, code, http.StatusOK)
		}
	}
}

func TestLoginRateLimitKeysForwardedClientBehindTrustedProxy(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newProxiedAuthServer(users, "192.0.2.0/24")
	const proxy = "192.0.2.1:9000"
	const wrong = `{"email":"ada@example.com","password":"wrong password!"}`

	var blocked bool
	for range 50 {
		if loginVia(t, handler, proxy, "203.0.113.10", wrong).Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("one forwarded client was never rate limited")
	}

	other := loginVia(t, handler, proxy, "203.0.113.20",
		`{"email":"ada@example.com","password":"correct horse battery"}`)
	if other.Code == http.StatusTooManyRequests {
		t.Fatal("a different forwarded client was locked out by another client's attempts")
	}
	if other.Code != http.StatusOK {
		t.Fatalf("the second forwarded client got status %d, want %d", other.Code, http.StatusOK)
	}
}

func TestLoginRateLimitResistsForwardedForSpoofingBehindProxy(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newProxiedAuthServer(users, "10.0.0.0/8")
	const proxy = "10.0.0.2:5000"
	const realClientAppendedByProxy = "203.0.113.50"
	const wrong = `{"email":"ada@example.com","password":"wrong password!"}`

	var blocked bool
	for i := range 50 {
		spoofedHead := fmt.Sprintf("%d.%d.%d.%d", i%250+1, i%100+1, i%50+1, i%200+1)
		forwardedFor := spoofedHead + ", " + realClientAppendedByProxy
		if loginVia(t, handler, proxy, forwardedFor, wrong).Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("rotating a spoofed X-Forwarded-For head bypassed the per-client rate limit")
	}
}

func TestLoginRateLimitIgnoresForwardedForWithoutTrustedProxy(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newAuthServer(users)
	const proxy = "198.51.100.30:7000"
	const wrong = `{"email":"ada@example.com","password":"wrong password!"}`

	var blocked bool
	for i := range 50 {
		spoofed := fmt.Sprintf("203.0.113.%d", i%200+1)
		if loginVia(t, handler, proxy, spoofed, wrong).Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("rotating X-Forwarded-For bypassed the rate limit with no trusted proxy configured")
	}
}

func TestLoginRateLimitHandlesAddressWithoutPort(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newAuthServer(users)

	recorder := loginFrom(t, handler, "198.51.100.10",
		`{"email":"ada@example.com","password":"correct horse battery"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for a RemoteAddr without a port", recorder.Code, http.StatusOK)
	}
}

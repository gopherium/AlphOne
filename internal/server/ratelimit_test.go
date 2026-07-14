// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func loginFrom(t *testing.T, handler http.Handler, ip, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = ip
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
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
	if last.Header().Get("Retry-After") == "" {
		t.Error("rate-limit response is missing a Retry-After header")
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

// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/gouncer/authkit/testkit"
)

const testPassword = "correct horse battery"

// newFakeUserStore returns an empty in-memory user store double.
func newFakeUserStore() *testkit.Store {
	return testkit.NewStore()
}

// addAda stores the default test user.
func addAda(t *testing.T, store *testkit.Store) {
	t.Helper()
	store.AddUser(t, "ada@example.com", "Ada Lovelace", testPassword)
}

// doLogin posts credentials to the login route.
func doLogin(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// sessionCookie returns the response's session cookie, failing without one.
func sessionCookie(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "__Host-alphone_session" {
			return cookie
		}
	}
	t.Fatal("no alphone_session cookie in the response")
	return nil
}

// loginCookie logs the default test user in and returns the issued cookie.
func loginCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	recorder := doLogin(t, handler, `{"email":"ada@example.com","password":"correct horse battery"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", recorder.Code, http.StatusOK)
	}
	return sessionCookie(t, recorder)
}

// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/event"
)

const testPassword = "correct horse battery"

// secondUserEmail addresses the second user of a two user store.
const secondUserEmail = "grace@example.com"

// newFakeUserStore returns an empty in-memory user store double.
func newFakeUserStore() *testkit.Store {
	return testkit.NewStore()
}

// addAda stores the default test user and returns it.
func addAda(t *testing.T, store *testkit.Store) gouncer.User {
	t.Helper()
	return store.AddUser(t, "ada@example.com", "Ada Lovelace", testPassword)
}

// waitForSubscribers blocks until the hub holds count subscriptions.
func waitForSubscribers(t *testing.T, hub *event.Hub, count int) {
	t.Helper()
	for range 200 {
		if hub.Subscribers() == count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("hub holds %d subscriptions, want %d", hub.Subscribers(), count)
}

// twoUserStore returns a store holding the default test user and a second one, beside the first.
func twoUserStore(t *testing.T) (*testkit.Store, gouncer.User) {
	t.Helper()
	store := newFakeUserStore()
	first := addAda(t, store)
	store.AddUser(t, secondUserEmail, "Grace Hopper", testPassword)
	return store, first
}

// twoUserCookies logs both users of a two user store in and returns a cookie each.
func twoUserCookies(t *testing.T, handler http.Handler) [2]*http.Cookie {
	t.Helper()
	second := doLogin(t, handler, `{"email":"`+secondUserEmail+`","password":"`+testPassword+`"}`)
	if second.Code != http.StatusOK {
		t.Fatalf("second login status = %d, want %d", second.Code, http.StatusOK)
	}
	return [2]*http.Cookie{loginCookie(t, handler), sessionCookie(t, second)}
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

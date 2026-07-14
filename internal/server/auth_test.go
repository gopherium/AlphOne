// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/server"
)

var (
	_ gouncer.Store = (*postgres.UserStore)(nil)
	_ gouncer.Store = (*fakeUserStore)(nil)
)

type fakeUserStore struct {
	users          map[uuid.UUID]gouncer.User
	sessions       map[string]gouncer.Session
	lookupErr      error
	sessionErr     error
	createErr      error
	deleteErr      error
	listUsersErr   error
	createUserErr  error
	setDisabledErr error
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		users:    map[uuid.UUID]gouncer.User{},
		sessions: map[string]gouncer.Session{},
	}
}

func (f *fakeUserStore) CreateUser(_ context.Context, u gouncer.User) error {
	if f.createUserErr != nil {
		return f.createUserErr
	}
	for _, existing := range f.users {
		if existing.Email == u.Email {
			return gouncer.ErrEmailTaken
		}
	}
	f.users[u.ID] = u
	return nil
}

func (f *fakeUserStore) ListUsers(_ context.Context) ([]gouncer.User, error) {
	if f.listUsersErr != nil {
		return nil, f.listUsersErr
	}
	users := slices.Collect(maps.Values(f.users))
	slices.SortFunc(users, func(a, b gouncer.User) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	for i := range users {
		users[i].PasswordHash = ""
	}
	return users, nil
}

func (f *fakeUserStore) SetUserDisabled(_ context.Context, id uuid.UUID, disabled bool) error {
	if f.setDisabledErr != nil {
		return f.setDisabledErr
	}
	u, ok := f.users[id]
	if !ok {
		return gouncer.ErrUserNotFound
	}
	u.Disabled = disabled
	f.users[id] = u
	if disabled {
		maps.DeleteFunc(f.sessions, func(_ string, s gouncer.Session) bool {
			return s.UserID == id
		})
	}
	return nil
}

func (f *fakeUserStore) UserByEmail(_ context.Context, email string) (gouncer.User, error) {
	if f.lookupErr != nil {
		return gouncer.User{}, f.lookupErr
	}
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return gouncer.User{}, gouncer.ErrUserNotFound
}

func (f *fakeUserStore) CreateSession(_ context.Context, s gouncer.Session) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.sessions[string(s.TokenHash)] = s
	return nil
}

func (f *fakeUserStore) UserBySession(_ context.Context, tokenHash []byte, now time.Time) (gouncer.User, error) {
	if f.sessionErr != nil {
		return gouncer.User{}, f.sessionErr
	}
	s, ok := f.sessions[string(tokenHash)]
	if !ok || !s.ExpiresAt.After(now) {
		return gouncer.User{}, gouncer.ErrSessionNotFound
	}
	u, ok := f.users[s.UserID]
	if !ok || u.Disabled {
		return gouncer.User{}, gouncer.ErrSessionNotFound
	}
	return u, nil
}

func (f *fakeUserStore) DeleteSession(_ context.Context, tokenHash []byte) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.sessions, string(tokenHash))
	return nil
}

func (f *fakeUserStore) addUser(t *testing.T) gouncer.User {
	t.Helper()
	return f.addNamedUser(t, "ada@example.com", "Ada Lovelace")
}

func (f *fakeUserStore) addNamedUser(t *testing.T, email, name string) gouncer.User {
	t.Helper()
	u, err := gouncer.NewUser(email, name, "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	f.users[u.ID] = u
	return u
}

func newAuthServer(users server.UserStore) http.Handler {
	return server.NewServer(server.Config{
		Contacts: newFakeContactStore(),
		Users:    users,
	})
}

func doLogin(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

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

func TestLoginIssuesASessionCookie(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newAuthServer(users)

	recorder := doLogin(t, handler, `{"email":" ADA@Example.com ","password":"correct horse battery"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	cookie := sessionCookie(t, recorder)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Errorf("cookie = %+v, want HttpOnly, Secure, SameSite=Lax, Path=/", cookie)
	}
	if _, ok := users.sessions[string(gouncer.HashToken(cookie.Value))]; !ok {
		t.Error("no session persisted for the issued cookie token")
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %q: %v", recorder.Body.String(), err)
	}
	if body["email"] != "ada@example.com" || body["name"] != "Ada Lovelace" {
		t.Errorf("body = %v, want the logged-in user's email and name", body)
	}
	if _, exposed := body["password_hash"]; exposed {
		t.Error("response exposes password_hash")
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		configure func(t *testing.T, users *fakeUserStore)
		body      string
		want      int
	}{
		"wrong password": {
			configure: func(t *testing.T, users *fakeUserStore) {
				users.addUser(t)
			},
			body: `{"email":"ada@example.com","password":"wrong password!"}`,
			want: http.StatusUnauthorized,
		},
		"unknown email": {
			configure: func(t *testing.T, users *fakeUserStore) {},
			body:      `{"email":"nobody@example.com","password":"correct horse battery"}`,
			want:      http.StatusUnauthorized,
		},
		"disabled user": {
			configure: func(t *testing.T, users *fakeUserStore) {
				u := users.addUser(t)
				u.Disabled = true
				users.users[u.ID] = u
			},
			body: `{"email":"ada@example.com","password":"correct horse battery"}`,
			want: http.StatusUnauthorized,
		},
		"malformed body": {
			configure: func(t *testing.T, users *fakeUserStore) {},
			body:      `{"email":`,
			want:      http.StatusBadRequest,
		},
		"trailing content": {
			configure: func(t *testing.T, users *fakeUserStore) {},
			body:      `{"email":"ada@example.com","password":"correct horse battery"}{}`,
			want:      http.StatusBadRequest,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			users := newFakeUserStore()
			tc.configure(t, users)
			handler := newAuthServer(users)

			recorder := doLogin(t, handler, tc.body)

			if recorder.Code != tc.want {
				t.Errorf("status = %d, want %d", recorder.Code, tc.want)
			}
			if recorder.Code != http.StatusOK {
				for _, cookie := range recorder.Result().Cookies() {
					if cookie.Name == "__Host-alphone_session" && cookie.MaxAge >= 0 {
						t.Error("failed login issued a session cookie")
					}
				}
			}
		})
	}
}

func TestLoginReportsStoreFailures(t *testing.T) {
	t.Parallel()

	t.Run("lookup failure", func(t *testing.T) {
		t.Parallel()

		users := newFakeUserStore()
		users.lookupErr = context.DeadlineExceeded
		handler := newAuthServer(users)

		recorder := doLogin(t, handler, `{"email":"ada@example.com","password":"correct horse battery"}`)

		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
	})

	t.Run("session creation failure", func(t *testing.T) {
		t.Parallel()

		users := newFakeUserStore()
		users.addUser(t)
		users.createErr = context.DeadlineExceeded
		handler := newAuthServer(users)

		recorder := doLogin(t, handler, `{"email":"ada@example.com","password":"correct horse battery"}`)

		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
	})
}

func loginCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	recorder := doLogin(t, handler, `{"email":"ada@example.com","password":"correct horse battery"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", recorder.Code, http.StatusOK)
	}
	return sessionCookie(t, recorder)
}

func TestSessionEndpointReportsTheLoggedInUser(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newAuthServer(users)
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %q: %v", recorder.Body.String(), err)
	}
	if body["email"] != "ada@example.com" {
		t.Errorf("body = %v, want the session user", body)
	}
}

func TestSessionEndpointRejectsMissingOrDeadSessions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		request func(t *testing.T, handler http.Handler) *http.Request
		want    int
	}{
		"no cookie": {
			request: func(_ *testing.T, _ http.Handler) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
			},
			want: http.StatusUnauthorized,
		},
		"unknown token": {
			request: func(_ *testing.T, _ http.Handler) *http.Request {
				request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
				request.AddCookie(&http.Cookie{Name: "__Host-alphone_session", Value: "forged"})
				return request
			},
			want: http.StatusUnauthorized,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			users := newFakeUserStore()
			users.addUser(t)
			handler := newAuthServer(users)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, tc.request(t, handler))

			if recorder.Code != tc.want {
				t.Errorf("status = %d, want %d", recorder.Code, tc.want)
			}
		})
	}
}

func TestSessionEndpointReportsStoreFailure(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newAuthServer(users)
	cookie := loginCookie(t, handler)
	users.sessionErr = context.DeadlineExceeded

	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestLogoutDeletesTheSessionAndClearsTheCookie(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	users.addUser(t)
	handler := newAuthServer(users)
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if len(users.sessions) != 0 {
		t.Error("session survived logout")
	}
	cleared := sessionCookie(t, recorder)
	if cleared.MaxAge >= 0 {
		t.Errorf("cleared cookie MaxAge = %d, want negative", cleared.MaxAge)
	}

	t.Run("without a cookie", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNoContent {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusNoContent)
		}
	})

	t.Run("store failure", func(t *testing.T) {
		users.deleteErr = context.DeadlineExceeded
		defer func() { users.deleteErr = nil }()
		request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		request.AddCookie(loginCookie(t, handler))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
	})
}

// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/server"
)

var errTokenBackend = errors.New("token backend unavailable")

type fakeTokenStore struct {
	tokens  map[string]apitoken.Token
	err     error
	touched []uuid.UUID
}

func newFakeTokenStore() *fakeTokenStore {
	return &fakeTokenStore{tokens: map[string]apitoken.Token{}}
}

func (s *fakeTokenStore) ByHash(_ context.Context, hash string) (apitoken.Token, error) {
	if s.err != nil {
		return apitoken.Token{}, s.err
	}
	token, ok := s.tokens[hash]
	if !ok {
		return apitoken.Token{}, apitoken.ErrNotFound
	}
	return token, nil
}

func (s *fakeTokenStore) TouchLastUsed(_ context.Context, id uuid.UUID, _ time.Time) error {
	s.touched = append(s.touched, id)
	return nil
}

// newTokenServer returns a server accepting bearer tokens, the token store
// behind it, and a secret minted for the default test user.
func newTokenServer(t *testing.T) (http.Handler, *fakeTokenStore, *testkit.Store, string) {
	t.Helper()
	users := newFakeUserStore()
	ada := addAda(t, users)
	tokens := newFakeTokenStore()
	minted, err := apitoken.Mint(ada.ID, "n8n production")
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	tokens.tokens[minted.Token.Hash] = minted.Token
	handler := newGraphServer(t, server.Config{
		Contacts: newFakeContactStore(),
		Tasks:    newFakeTaskStore(),
		Users:    users,
		Tokens:   tokens,
		Version:  "9.9.9",
		Plugins: map[string]http.Handler{
			"echo": echoHandler(http.StatusOK, "plugin says hi"),
		},
	})
	return handler, tokens, users, minted.Secret
}

// getWithBearer issues a GET carrying the given bearer credential.
func getWithBearer(handler http.Handler, path, secret string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestBearerTokenAuthenticatesACoreRoute(t *testing.T) {
	t.Parallel()

	handler, _, _, secret := newTokenServer(t)

	recorder := getWithBearer(handler, "/api/version", secret)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestBearerTokenAuthenticatesAPluginRoute(t *testing.T) {
	t.Parallel()

	handler, _, _, secret := newTokenServer(t)

	recorder := getWithBearer(handler, "/api/plugins/echo/conversations", secret)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestBearerTokenCarriesTheOwnersIdentity(t *testing.T) {
	t.Parallel()

	handler, _, _, secret := newTokenServer(t)

	recorder := getWithBearer(handler, "/api/auth/session", secret)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var identity struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&identity); err != nil {
		t.Fatalf("decoding identity: %v", err)
	}
	if identity.Email != "ada@example.com" {
		t.Errorf("email = %q, want the token owner's email", identity.Email)
	}
	if identity.Name != "Ada Lovelace" {
		t.Errorf("name = %q, want the token owner's name", identity.Name)
	}
}

func TestBearerTokenStampsTaskOrigin(t *testing.T) {
	t.Parallel()

	srv, _, _, secret := newTokenServer(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+secret)
		srv.ServeHTTP(w, r)
	})

	recorder := doRequest(t, handler, http.MethodPost, "/api/tasks",
		`{"title":"Follow up with Maria Perez","due_on":"2026-08-01"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.OriginSource == nil || *got.OriginSource != "token:n8n production" {
		t.Errorf("origin_source = %v, want the stamped token name", got.OriginSource)
	}
	if got.OriginEventID != nil {
		t.Errorf("origin_event_id = %v, want null", got.OriginEventID)
	}
}

func TestBearerTokenIsRejectedWhenUnknown(t *testing.T) {
	t.Parallel()

	handler, _, _, _ := newTokenServer(t)

	recorder := getWithBearer(handler, "/api/version", "a1_never_minted")

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestBearerTokenIsRejectedForADisabledUser(t *testing.T) {
	t.Parallel()

	handler, _, users, secret := newTokenServer(t)
	ada, err := users.UserByEmail(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if err := users.SetUserDisabled(t.Context(), ada.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}

	recorder := getWithBearer(handler, "/api/version", secret)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestBearerTokenIsRejectedWhenTheOwnerIsGone(t *testing.T) {
	t.Parallel()

	handler, tokens, _, _ := newTokenServer(t)
	orphan, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "orphan")
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	tokens.tokens[orphan.Token.Hash] = orphan.Token

	recorder := getWithBearer(handler, "/api/version", orphan.Secret)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestBearerTokenReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	handler, tokens, _, secret := newTokenServer(t)
	tokens.err = errTokenBackend

	recorder := getWithBearer(handler, "/api/version", secret)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestBearerTokenReportsAUserLookupFailure(t *testing.T) {
	t.Parallel()

	handler, _, users, secret := newTokenServer(t)
	users.LookupErr = errTokenBackend

	recorder := getWithBearer(handler, "/api/version", secret)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestBearerTokenRecordsItsLastUse(t *testing.T) {
	t.Parallel()

	handler, tokens, _, secret := newTokenServer(t)

	if recorder := getWithBearer(handler, "/api/version", secret); recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if len(tokens.touched) != 1 {
		t.Errorf("touched %d tokens, want 1", len(tokens.touched))
	}
}

func TestUnusableAuthorizationHeadersFallBackToTheSession(t *testing.T) {
	t.Parallel()

	handler, _, _, _ := newTokenServer(t)
	cookie := loginCookie(t, handler)

	for name, header := range map[string]string{
		"empty credential":   "Bearer ",
		"another scheme":     "Basic YWRhOnNlY3JldA==",
		"no scheme at all":   "a1_looks_like_a_token",
		"bearer with no gap": "Bearer",
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/version", nil)
		request.Header.Set("Authorization", header)
		request.AddCookie(cookie)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Errorf("%s with a session cookie status = %d, want %d", name, recorder.Code, http.StatusOK)
		}
	}
}

func TestUnusableAuthorizationHeadersStayUnauthorizedWithoutASession(t *testing.T) {
	t.Parallel()

	handler, _, _, _ := newTokenServer(t)

	for name, header := range map[string]string{
		"empty credential": "Bearer ",
		"another scheme":   "Basic YWRhOnNlY3JldA==",
		"no scheme at all": "a1_looks_like_a_token",
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/version", nil)
		request.Header.Set("Authorization", header)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s status = %d, want %d", name, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestSessionCookieStillAuthenticatesAlongsideTokens(t *testing.T) {
	t.Parallel()

	handler, _, _, _ := newTokenServer(t)
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

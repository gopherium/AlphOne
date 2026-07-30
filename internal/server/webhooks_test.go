// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/internal/webhook"
)

var errWebhookBackend = errors.New("webhook backend unavailable")

type fakeWebhookStore struct {
	subs      map[uuid.UUID]webhook.Subscription
	createErr error
	listErr   error
	deleteErr error
}

func newFakeWebhookStore() *fakeWebhookStore {
	return &fakeWebhookStore{subs: map[uuid.UUID]webhook.Subscription{}}
}

func (s *fakeWebhookStore) CreateSubscription(_ context.Context, sub webhook.Subscription) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.subs[sub.ID] = sub
	return nil
}

func (s *fakeWebhookStore) ListSubscriptionsForUser(
	_ context.Context,
	userID uuid.UUID,
) ([]webhook.Subscription, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var found []webhook.Subscription
	for _, sub := range s.subs {
		if sub.UserID == userID {
			found = append(found, sub)
		}
	}
	return found, nil
}

func (s *fakeWebhookStore) DeleteSubscription(_ context.Context, userID, id uuid.UUID) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	sub, ok := s.subs[id]
	if !ok || sub.UserID != userID {
		return webhook.ErrNotFound
	}
	delete(s.subs, id)
	return nil
}

// newWebhookServer returns a server with the webhook API mounted, the store
// behind it, and a session cookie for the default test user.
func newWebhookServer(t *testing.T) (http.Handler, *fakeWebhookStore, *http.Cookie) {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	store := newFakeWebhookStore()
	handler := server.NewServer(server.Config{
		Contacts: newFakeContactStore(),
		Users:    users,
		Webhooks: store,
	})
	return handler, store, loginCookie(t, handler)
}

// doWebhook issues an authenticated request to the webhook API.
func doWebhook(handler http.Handler, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestWebhookCreateReturnsTheSecretOnce(t *testing.T) {
	t.Parallel()

	handler, store, cookie := newWebhookServer(t)

	recorder := doWebhook(handler, cookie, http.MethodPost, "/api/webhooks",
		`{"url":"https://example.com/hook","events":["task.created"]}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	var created struct {
		ID     uuid.UUID `json:"id"`
		URL    string    `json:"url"`
		Events []string  `json:"events"`
		Secret string    `json:"secret"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&created); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if !strings.HasPrefix(created.Secret, webhook.SecretPrefix) {
		t.Errorf("secret = %q, want the signing secret", created.Secret)
	}
	if created.URL != "https://example.com/hook" {
		t.Errorf("url = %q, want the subscribed url", created.URL)
	}
	if len(store.subs) != 1 {
		t.Errorf("stored %d subscriptions, want 1", len(store.subs))
	}

	listing := doWebhook(handler, cookie, http.MethodGet, "/api/webhooks", "")
	if strings.Contains(listing.Body.String(), webhook.SecretPrefix) {
		t.Errorf("listing leaks the secret: %s", listing.Body)
	}
}

func TestWebhookListCarriesWhatATriggerMatchesOn(t *testing.T) {
	t.Parallel()

	handler, _, cookie := newWebhookServer(t)
	if r := doWebhook(handler, cookie, http.MethodPost, "/api/webhooks",
		`{"url":"https://example.com/hook","events":["task.created","contact.created"]}`); r.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", r.Code, http.StatusCreated)
	}

	recorder := doWebhook(handler, cookie, http.MethodGet, "/api/webhooks", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var listing struct {
		Webhooks []struct {
			ID     uuid.UUID `json:"id"`
			URL    string    `json:"url"`
			Events []string  `json:"events"`
		} `json:"webhooks"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&listing); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(listing.Webhooks) != 1 {
		t.Fatalf("got %d webhooks, want 1", len(listing.Webhooks))
	}
	got := listing.Webhooks[0]
	if got.ID == uuid.Nil || got.URL == "" || len(got.Events) != 2 {
		t.Errorf("listing = %+v, want id, url and events for a trigger to match on", got)
	}
}

func TestWebhookCreateRejectsBadInput(t *testing.T) {
	t.Parallel()

	handler, _, cookie := newWebhookServer(t)

	for name, body := range map[string]string{
		"malformed json": `{`,
		"bad url":        `{"url":"not-a-url","events":["task.created"]}`,
		"no events":      `{"url":"https://example.com/h","events":[]}`,
		"unknown event":  `{"url":"https://example.com/h","events":["task.deleted"]}`,
	} {
		recorder := doWebhook(handler, cookie, http.MethodPost, "/api/webhooks", body)

		if recorder.Code < 400 || recorder.Code >= 500 {
			t.Errorf("%s: status = %d, want a client error", name, recorder.Code)
		}
	}
}

func TestWebhookDeleteRevokesTheSubscription(t *testing.T) {
	t.Parallel()

	handler, store, cookie := newWebhookServer(t)
	created := doWebhook(handler, cookie, http.MethodPost, "/api/webhooks",
		`{"url":"https://example.com/hook","events":["task.created"]}`)
	var body struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	recorder := doWebhook(handler, cookie, http.MethodDelete, "/api/webhooks/"+body.ID.String(), "")

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if len(store.subs) != 0 {
		t.Errorf("stored %d subscriptions after delete, want 0", len(store.subs))
	}
}

func TestWebhookDeleteReportsAnUnknownSubscription(t *testing.T) {
	t.Parallel()

	handler, _, cookie := newWebhookServer(t)

	recorder := doWebhook(handler, cookie, http.MethodDelete,
		"/api/webhooks/"+uuid.Must(uuid.NewV7()).String(), "")

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestWebhookDeleteRejectsAMalformedID(t *testing.T) {
	t.Parallel()

	handler, _, cookie := newWebhookServer(t)

	recorder := doWebhook(handler, cookie, http.MethodDelete, "/api/webhooks/not-a-uuid", "")

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestWebhookRoutesNeedACredential(t *testing.T) {
	t.Parallel()

	handler, _, _ := newWebhookServer(t)

	for _, target := range []struct{ method, path string }{
		{http.MethodGet, "/api/webhooks"},
		{http.MethodPost, "/api/webhooks"},
		{http.MethodDelete, "/api/webhooks/" + uuid.Nil.String()},
	} {
		request := httptest.NewRequest(target.method, target.path, strings.NewReader("{}"))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want %d", target.method, target.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestWebhookReportsStoreFailures(t *testing.T) {
	t.Parallel()

	handler, store, cookie := newWebhookServer(t)
	store.createErr = errWebhookBackend
	store.listErr = errWebhookBackend
	store.deleteErr = errWebhookBackend

	created := doWebhook(handler, cookie, http.MethodPost, "/api/webhooks",
		`{"url":"https://example.com/hook","events":["task.created"]}`)
	if created.Code != http.StatusInternalServerError {
		t.Errorf("create status = %d, want %d", created.Code, http.StatusInternalServerError)
	}
	listed := doWebhook(handler, cookie, http.MethodGet, "/api/webhooks", "")
	if listed.Code != http.StatusInternalServerError {
		t.Errorf("list status = %d, want %d", listed.Code, http.StatusInternalServerError)
	}
	deleted := doWebhook(handler, cookie, http.MethodDelete,
		"/api/webhooks/"+uuid.Must(uuid.NewV7()).String(), "")
	if deleted.Code != http.StatusInternalServerError {
		t.Errorf("delete status = %d, want %d", deleted.Code, http.StatusInternalServerError)
	}
}

func TestWebhookListIsScopedToTheCaller(t *testing.T) {
	t.Parallel()

	handler, store, cookie := newWebhookServer(t)
	stranger, err := webhook.NewSubscription(
		uuid.Must(uuid.NewV7()), "https://example.com/theirs", []event.Name{event.TaskCreated},
	)
	if err != nil {
		t.Fatalf("NewSubscription() error = %v, want nil", err)
	}
	store.subs[stranger.ID] = stranger

	recorder := doWebhook(handler, cookie, http.MethodGet, "/api/webhooks", "")

	if strings.Contains(recorder.Body.String(), "theirs") {
		t.Errorf("listing leaks another user's subscription: %s", recorder.Body)
	}
}

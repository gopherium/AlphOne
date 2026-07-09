// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/server"
)

var (
	_ server.ContactStore = (*postgres.ContactStore)(nil)
	_ server.ContactStore = (*fakeContactStore)(nil)
)

type fakeContactStore struct {
	contacts  map[uuid.UUID]contact.Contact
	createErr error
	getErr    error
}

func newFakeContactStore() *fakeContactStore {
	return &fakeContactStore{contacts: map[uuid.UUID]contact.Contact{}}
}

func (f *fakeContactStore) Create(_ context.Context, c contact.Contact) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.contacts[c.ID] = c
	return nil
}

func (f *fakeContactStore) Get(_ context.Context, id uuid.UUID) (contact.Contact, error) {
	if f.getErr != nil {
		return contact.Contact{}, f.getErr
	}
	c, ok := f.contacts[id]
	if !ok {
		return contact.Contact{}, contact.ErrNotFound
	}
	return c, nil
}

type contactBody struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type errorBody struct {
	Error string `json:"error"`
}

func doRequest(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeBody[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(recorder.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding response %q: %v", recorder.Body.String(), err)
	}
	return v
}

func TestPluginRoutesAreMountedUnderTheirNamespace(t *testing.T) {
	t.Parallel()

	echoPath := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.URL.Path))
	})
	srv := server.NewServer(newFakeContactStore(), map[string]http.Handler{"demo": echoPath})

	recorder := doRequest(t, srv, http.MethodGet, "/api/plugins/demo/ping", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != "/ping" {
		t.Errorf("plugin saw path %q, want %q stripped of its namespace", got, "/ping")
	}
}

func TestCreateContact(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	srv := server.NewServer(store, nil)

	recorder := doRequest(t, srv, http.MethodPost, "/api/contacts", `{"name":"  María Pérez  "}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}
	got := decodeBody[contactBody](t, recorder)
	if got.Name != "María Pérez" {
		t.Errorf("name = %q, want %q", got.Name, "María Pérez")
	}
	if got.ID == uuid.Nil {
		t.Error("id is uuid.Nil, want a generated UUID")
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at is zero, want a timestamp")
	}
	stored, ok := store.contacts[got.ID]
	if !ok || stored.Name != got.Name {
		t.Errorf("stored contact = %+v, want name %q under id %s", stored, got.Name, got.ID)
	}
}

func TestCreateContactRejectsInvalidBody(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body       string
		wantStatus int
		wantError  string
	}{
		"malformed json": {body: `{"name":`, wantStatus: http.StatusBadRequest, wantError: "malformed json"},
		"blank name": {
			body:       `{"name":" \t "}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "contact: empty name",
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			srv := server.NewServer(newFakeContactStore(), nil)

			recorder := doRequest(t, srv, http.MethodPost, "/api/contacts", tc.body)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
			if got := decodeBody[errorBody](t, recorder); got.Error != tc.wantError {
				t.Errorf("error = %q, want %q", got.Error, tc.wantError)
			}
		})
	}
}

func TestCreateContactHidesStoreFailure(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	store.createErr = errors.New("connection refused to 10.0.0.7")
	srv := server.NewServer(store, nil)

	recorder := doRequest(t, srv, http.MethodPost, "/api/contacts", `{"name":"María"}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if got := decodeBody[errorBody](t, recorder); got.Error != "internal error" {
		t.Errorf("error = %q, want %q without internals leaking", got.Error, "internal error")
	}
}

func TestGetContact(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	maria, err := contact.New("María Pérez")
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	store.contacts[maria.ID] = maria
	srv := server.NewServer(store, nil)

	recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/"+maria.ID.String(), "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[contactBody](t, recorder)
	if got.ID != maria.ID || got.Name != maria.Name {
		t.Errorf("body = %+v, want id %s and name %q", got, maria.ID, maria.Name)
	}
}

func TestGetContactNormalizesTimestampToUTC(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	maria, err := contact.New("María Pérez")
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	maria.CreatedAt = maria.CreatedAt.In(time.FixedZone("CET", 2*60*60))
	store.contacts[maria.ID] = maria
	srv := server.NewServer(store, nil)

	recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/"+maria.ID.String(), "")

	got := decodeBody[contactBody](t, recorder)
	if got.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at location = %v, want UTC", got.CreatedAt.Location())
	}
}

func TestGetContactErrors(t *testing.T) {
	t.Parallel()

	t.Run("malformed id", func(t *testing.T) {
		t.Parallel()

		srv := server.NewServer(newFakeContactStore(), nil)

		recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/not-a-uuid", "")

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
	})

	t.Run("unknown contact", func(t *testing.T) {
		t.Parallel()

		srv := server.NewServer(newFakeContactStore(), nil)

		recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/"+uuid.Must(uuid.NewV7()).String(), "")

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
		if got := decodeBody[errorBody](t, recorder); got.Error != "contact: not found" {
			t.Errorf("error = %q, want %q", got.Error, "contact: not found")
		}
	})

	t.Run("store failure", func(t *testing.T) {
		t.Parallel()

		store := newFakeContactStore()
		store.getErr = errors.New("connection refused to 10.0.0.7")
		srv := server.NewServer(store, nil)

		recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/"+uuid.Must(uuid.NewV7()).String(), "")

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
		if got := decodeBody[errorBody](t, recorder); got.Error != "internal error" {
			t.Errorf("error = %q, want %q without internals leaking", got.Error, "internal error")
		}
	})
}

// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
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
	listErr   error
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

func (f *fakeContactStore) ListContacts(
	_ context.Context, _, _ string, afterName string, afterID uuid.UUID, limit int,
) ([]contact.Contact, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	all := make([]contact.Contact, 0, len(f.contacts))
	for _, c := range f.contacts {
		all = append(all, c)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Name != all[j].Name {
			return all[i].Name < all[j].Name
		}
		return all[i].ID.String() < all[j].ID.String()
	})
	page := make([]contact.Contact, 0, limit)
	for _, c := range all {
		if c.Name < afterName || (c.Name == afterName && c.ID.String() <= afterID.String()) {
			continue
		}
		page = append(page, c)
		if len(page) == limit {
			break
		}
	}
	return page, nil
}

type contactListBody struct {
	Contacts   []contactBody `json:"contacts"`
	NextCursor *string       `json:"next_cursor"`
}

func seedContacts(t *testing.T, store *fakeContactStore, names ...string) {
	t.Helper()
	for _, name := range names {
		c, err := contact.New(name)
		if err != nil {
			t.Fatalf("contact.New(%q) error = %v, want nil", name, err)
		}
		if err := store.Create(t.Context(), c); err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
	}
}

func TestListContactsEndpointPaginates(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	seedContacts(t, store, "Ana", "Bruno", "Carla")
	srv := authedContactServer(t, store, nil)

	full := doRequest(t, srv, http.MethodGet, "/api/contacts", "")
	if full.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", full.Code, http.StatusOK)
	}
	fullPage := decodeBody[contactListBody](t, full)
	if len(fullPage.Contacts) != 3 || fullPage.NextCursor != nil {
		t.Fatalf("default page = %d contacts with cursor %v, want all 3 and no cursor",
			len(fullPage.Contacts), fullPage.NextCursor)
	}

	first := doRequest(t, srv, http.MethodGet, "/api/contacts?limit=2", "")
	firstPage := decodeBody[contactListBody](t, first)
	if len(firstPage.Contacts) != 2 || firstPage.NextCursor == nil {
		t.Fatalf("first page = %d contacts with cursor %v, want 2 and a cursor",
			len(firstPage.Contacts), firstPage.NextCursor)
	}
	if firstPage.Contacts[0].Name != "Ana" || firstPage.Contacts[1].Name != "Bruno" {
		t.Errorf("first page = %q, %q, want Ana, Bruno", firstPage.Contacts[0].Name, firstPage.Contacts[1].Name)
	}

	second := doRequest(t, srv, http.MethodGet, "/api/contacts?limit=2&cursor="+*firstPage.NextCursor, "")
	secondPage := decodeBody[contactListBody](t, second)
	if len(secondPage.Contacts) != 1 || secondPage.NextCursor != nil {
		t.Fatalf("second page = %d contacts with cursor %v, want 1 and no cursor",
			len(secondPage.Contacts), secondPage.NextCursor)
	}
	if secondPage.Contacts[0].Name != "Carla" {
		t.Errorf("second page contact = %q, want Carla", secondPage.Contacts[0].Name)
	}
}

func TestListContactsEndpointRoundTripsUnicodeCursors(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	seedContacts(t, store, "María Pérez", "Zoe")
	srv := authedContactServer(t, store, nil)

	first := doRequest(t, srv, http.MethodGet, "/api/contacts?limit=1", "")
	firstPage := decodeBody[contactListBody](t, first)
	if firstPage.NextCursor == nil {
		t.Fatal("first page cursor = nil, want a cursor")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*firstPage.NextCursor)
	if err != nil {
		t.Fatalf("decoding cursor: %v", err)
	}
	var cursor struct {
		Name string    `json:"name"`
		ID   uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		t.Fatalf("unmarshaling cursor %q: %v", decoded, err)
	}
	if cursor.Name != "María Pérez" {
		t.Errorf("cursor name = %q, want %q", cursor.Name, "María Pérez")
	}

	second := doRequest(t, srv, http.MethodGet, "/api/contacts?limit=1&cursor="+*firstPage.NextCursor, "")
	secondPage := decodeBody[contactListBody](t, second)
	if len(secondPage.Contacts) != 1 || secondPage.Contacts[0].Name != "Zoe" {
		t.Fatalf("second page = %+v, want Zoe", secondPage.Contacts)
	}
}

func TestListContactsEndpointRejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"limit zero":        "/api/contacts?limit=0",
		"limit too big":     "/api/contacts?limit=201",
		"limit junk":        "/api/contacts?limit=abc",
		"cursor bad base64": "/api/contacts?cursor=%21%21%21",
		"cursor bad json":   "/api/contacts?cursor=" + base64.RawURLEncoding.EncodeToString([]byte("not json")),
	}

	for testName, target := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			srv := authedContactServer(t, newFakeContactStore(), nil)

			recorder := doRequest(t, srv, http.MethodGet, target, "")

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestListContactsEndpointReportsStoreFailure(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	store.listErr = errors.New("boom")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodGet, "/api/contacts", "")

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
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

func authedContactServer(t *testing.T, store server.ContactStore, plugins map[string]http.Handler) http.Handler {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	srv := server.NewServer(server.Config{Contacts: store, Users: users, Plugins: plugins})
	cookie := loginCookie(t, srv)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		srv.ServeHTTP(w, r)
	})
}

func TestPluginRoutesAreMountedUnderTheirNamespace(t *testing.T) {
	t.Parallel()

	echoPath := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.URL.Path))
	})
	srv := authedContactServer(t, newFakeContactStore(), map[string]http.Handler{"demo": echoPath})

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
	srv := authedContactServer(t, store, nil)

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

			srv := authedContactServer(t, newFakeContactStore(), nil)

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
	srv := authedContactServer(t, store, nil)

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
	srv := authedContactServer(t, store, nil)

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
	srv := authedContactServer(t, store, nil)

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

		srv := authedContactServer(t, newFakeContactStore(), nil)

		recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/not-a-uuid", "")

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
	})

	t.Run("unknown contact", func(t *testing.T) {
		t.Parallel()

		srv := authedContactServer(t, newFakeContactStore(), nil)

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
		srv := authedContactServer(t, store, nil)

		recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/"+uuid.Must(uuid.NewV7()).String(), "")

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
		if got := decodeBody[errorBody](t, recorder); got.Error != "internal error" {
			t.Errorf("error = %q, want %q without internals leaking", got.Error, "internal error")
		}
	})
}

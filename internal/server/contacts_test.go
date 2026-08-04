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
	"net/url"
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
	contacts       map[uuid.UUID]contact.Contact
	identities     map[uuid.UUID][]contact.Identity
	createErr      error
	getErr         error
	listErr        error
	identitiesErr  error
	renameErr      error
	addIdentityErr error
	lastQuery      string
	lastDigits     string
}

func newFakeContactStore() *fakeContactStore {
	return &fakeContactStore{
		contacts:   map[uuid.UUID]contact.Contact{},
		identities: map[uuid.UUID][]contact.Identity{},
	}
}

func (f *fakeContactStore) ListContactIdentities(
	_ context.Context, contactID uuid.UUID,
) ([]contact.Identity, error) {
	if f.identitiesErr != nil {
		return nil, f.identitiesErr
	}
	return f.identities[contactID], nil
}

func (f *fakeContactStore) identityOwner(channel contact.Channel, identifier string) (uuid.UUID, bool) {
	for ownerID, identities := range f.identities {
		for _, identity := range identities {
			if identity.Channel == channel && identity.Identifier == identifier {
				return ownerID, true
			}
		}
	}
	return uuid.Nil, false
}

func (f *fakeContactStore) AddIdentity(_ context.Context, identity contact.Identity) error {
	if f.addIdentityErr != nil {
		return f.addIdentityErr
	}
	if _, ok := f.contacts[identity.ContactID]; !ok {
		return contact.ErrNotFound
	}
	if ownerID, claimed := f.identityOwner(identity.Channel, identity.Identifier); claimed {
		return contact.IdentityExistsError{OwnerID: ownerID}
	}
	f.identities[identity.ContactID] = append(f.identities[identity.ContactID], identity)
	return nil
}

func (f *fakeContactStore) DeleteIdentity(_ context.Context, contactID, identityID uuid.UUID) error {
	for i, identity := range f.identities[contactID] {
		if identity.ID == identityID {
			f.identities[contactID] = append(
				f.identities[contactID][:i], f.identities[contactID][i+1:]...,
			)
			return nil
		}
	}
	return contact.ErrIdentityNotFound
}

func (f *fakeContactStore) CreateContactWithIdentities(
	_ context.Context, c contact.Contact, identities []contact.Identity,
) error {
	for _, identity := range identities {
		if ownerID, claimed := f.identityOwner(identity.Channel, identity.Identifier); claimed {
			return contact.IdentityExistsError{OwnerID: ownerID}
		}
	}
	f.contacts[c.ID] = c
	f.identities[c.ID] = identities
	return nil
}

func (f *fakeContactStore) byName(t *testing.T, name string) contact.Contact {
	t.Helper()
	for _, c := range f.contacts {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no contact named %q in the fake store", name)
	return contact.Contact{}
}

func seedIdentity(
	t *testing.T, store *fakeContactStore, contactID uuid.UUID, channel contact.Channel, identifier string,
) contact.Identity {
	t.Helper()
	identity, err := contact.NewIdentity(contactID, channel, identifier, "")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}
	store.identities[contactID] = append(store.identities[contactID], identity)
	return identity
}

func (f *fakeContactStore) RenameContact(
	_ context.Context, id uuid.UUID, name string,
) (contact.Contact, error) {
	if f.renameErr != nil {
		return contact.Contact{}, f.renameErr
	}
	c, ok := f.contacts[id]
	if !ok {
		return contact.Contact{}, contact.ErrNotFound
	}
	c.Name = name
	f.contacts[id] = c
	return c, nil
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
	_ context.Context, query, digits string, afterName string, afterID uuid.UUID, limit int,
) ([]contact.Contact, error) {
	f.lastQuery = query
	f.lastDigits = digits
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

type identityBody struct {
	ID          uuid.UUID `json:"id"`
	Channel     string    `json:"channel"`
	Identifier  string    `json:"identifier"`
	DisplayName string    `json:"display_name"`
}

type identityConflictBody struct {
	Error string       `json:"error"`
	Owner *contactBody `json:"owner"`
}

type contactDetailBody struct {
	ID         uuid.UUID      `json:"id"`
	Name       string         `json:"name"`
	Identities []identityBody `json:"identities"`
}

func TestGetContactIncludesIdentities(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	ada, err := contact.New("Ada")
	if err != nil {
		t.Fatalf("contact.New() error = %v, want nil", err)
	}
	if err := store.Create(t.Context(), ada); err != nil {
		t.Fatalf("seeding Ada: %v", err)
	}
	identity, err := contact.NewIdentity(ada.ID, "whatsapp", "184467235", "Ada L")
	if err != nil {
		t.Fatalf("contact.NewIdentity() error = %v, want nil", err)
	}
	store.identities[ada.ID] = []contact.Identity{identity}
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/"+ada.ID.String(), "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	detail := decodeBody[contactDetailBody](t, recorder)
	if detail.Name != "Ada" || len(detail.Identities) != 1 {
		t.Fatalf("detail = %+v, want Ada with one identity", detail)
	}
	got := detail.Identities[0]
	if got.Channel != "whatsapp" || got.Identifier != "184467235" || got.DisplayName != "Ada L" {
		t.Errorf("identity = %+v, want the seeded whatsapp identity", got)
	}
	if got.ID != identity.ID {
		t.Errorf("identity id = %s, want %s", got.ID, identity.ID)
	}
}

func TestContactIdentityCreateEndpoint(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	seedContacts(t, store, "María Pérez")
	maria := store.byName(t, "María Pérez")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPost, "/api/contacts/"+maria.ID.String()+"/identities",
		`{"channel": " Email ", "identifier": " Maria.Perez@Example.COM ", "display_name": " Work "}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	got := decodeBody[identityBody](t, recorder)
	if got.ID == uuid.Nil {
		t.Error("identity id = nil, want a generated id")
	}
	if got.Channel != "email" || got.Identifier != "maria.perez@example.com" || got.DisplayName != "Work" {
		t.Errorf("identity = %+v, want the normalized email identity", got)
	}
	if stored := store.identities[maria.ID]; len(stored) != 1 {
		t.Errorf("stored identities = %d, want 1", len(stored))
	}
}

func TestContactIdentityCreateNamesTheOwnerOnConflict(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	seedContacts(t, store, "María Pérez", "John Doe")
	maria := store.byName(t, "María Pérez")
	john := store.byName(t, "John Doe")
	seedIdentity(t, store, maria.ID, "email", "maria@example.com")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPost, "/api/contacts/"+john.ID.String()+"/identities",
		`{"channel": "email", "identifier": "maria@example.com"}`)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusConflict, recorder.Body)
	}
	conflict := decodeBody[identityConflictBody](t, recorder)
	if conflict.Error != "contact: identity already exists" {
		t.Errorf("error = %q, want the identity exists message", conflict.Error)
	}
	if conflict.Owner == nil || conflict.Owner.ID != maria.ID || conflict.Owner.Name != "María Pérez" {
		t.Errorf("owner = %+v, want María Pérez", conflict.Owner)
	}
}

func TestContactIdentityCreateConflictSurvivesAnOwnerLookupFailure(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	seedContacts(t, store, "María Pérez", "John Doe")
	maria := store.byName(t, "María Pérez")
	john := store.byName(t, "John Doe")
	seedIdentity(t, store, maria.ID, "email", "maria@example.com")
	store.getErr = errors.New("boom")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPost, "/api/contacts/"+john.ID.String()+"/identities",
		`{"channel": "email", "identifier": "maria@example.com"}`)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusConflict, recorder.Body)
	}
	conflict := decodeBody[identityConflictBody](t, recorder)
	if conflict.Owner != nil {
		t.Errorf("owner = %+v, want none when the lookup fails", conflict.Owner)
	}
}

func TestContactIdentityCreateValidation(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	seedContacts(t, store, "María Pérez")
	maria := store.byName(t, "María Pérez")

	tests := map[string]struct {
		path       string
		body       string
		wantStatus int
		wantError  string
	}{
		"channel outside the allowlist": {
			path:       "/api/contacts/" + maria.ID.String() + "/identities",
			body:       `{"channel": "whatsapp", "identifier": "184467235"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "contact: channel not writable",
		},
		"empty identifier": {
			path:       "/api/contacts/" + maria.ID.String() + "/identities",
			body:       `{"channel": "email", "identifier": " "}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "contact: empty identifier",
		},
		"phone with no digits": {
			path:       "/api/contacts/" + maria.ID.String() + "/identities",
			body:       `{"channel": "phone", "identifier": "no digits"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "contact: empty identifier",
		},
		"blank channel": {
			path:       "/api/contacts/" + maria.ID.String() + "/identities",
			body:       `{"channel": " ", "identifier": "184467235"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "contact: empty channel",
		},
		"unknown contact": {
			path:       "/api/contacts/" + uuid.Must(uuid.NewV7()).String() + "/identities",
			body:       `{"channel": "email", "identifier": "maria@example.com"}`,
			wantStatus: http.StatusNotFound,
			wantError:  "contact: not found",
		},
		"malformed contact id": {
			path:       "/api/contacts/not-a-uuid/identities",
			body:       `{"channel": "email", "identifier": "maria@example.com"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "malformed contact id",
		},
		"malformed json": {
			path:       "/api/contacts/" + maria.ID.String() + "/identities",
			body:       `{`,
			wantStatus: http.StatusBadRequest,
			wantError:  "malformed json",
		},
	}

	srv := authedContactServer(t, store, nil)
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			recorder := doRequest(t, srv, http.MethodPost, tc.path, tc.body)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", recorder.Code, tc.wantStatus, recorder.Body)
			}
			if got := decodeBody[errorBody](t, recorder); got.Error != tc.wantError {
				t.Errorf("error = %q, want %q", got.Error, tc.wantError)
			}
		})
	}
}

func TestContactIdentityCreateReportsStoreFailure(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	seedContacts(t, store, "María Pérez")
	maria := store.byName(t, "María Pérez")
	store.addIdentityErr = errors.New("boom")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPost, "/api/contacts/"+maria.ID.String()+"/identities",
		`{"channel": "email", "identifier": "maria@example.com"}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestContactIdentityDeleteEndpoint(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	seedContacts(t, store, "María Pérez", "John Doe")
	maria := store.byName(t, "María Pérez")
	john := store.byName(t, "John Doe")
	identity := seedIdentity(t, store, maria.ID, "phone", "+184467235")
	srv := authedContactServer(t, store, nil)

	t.Run("malformed contact id", func(t *testing.T) {
		recorder := doRequest(t, srv, http.MethodDelete,
			"/api/contacts/not-a-uuid/identities/"+identity.ID.String(), "")

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
	})

	t.Run("someone else's contact path misses", func(t *testing.T) {
		recorder := doRequest(t, srv, http.MethodDelete,
			"/api/contacts/"+john.ID.String()+"/identities/"+identity.ID.String(), "")

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
	})

	t.Run("the owner's path deletes", func(t *testing.T) {
		recorder := doRequest(t, srv, http.MethodDelete,
			"/api/contacts/"+maria.ID.String()+"/identities/"+identity.ID.String(), "")

		if recorder.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
		}
		if len(store.identities[maria.ID]) != 0 {
			t.Errorf("stored identities = %d, want 0", len(store.identities[maria.ID]))
		}
	})

	t.Run("a second delete misses", func(t *testing.T) {
		recorder := doRequest(t, srv, http.MethodDelete,
			"/api/contacts/"+maria.ID.String()+"/identities/"+identity.ID.String(), "")

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
	})

	t.Run("malformed identity id", func(t *testing.T) {
		recorder := doRequest(t, srv, http.MethodDelete,
			"/api/contacts/"+maria.ID.String()+"/identities/not-a-uuid", "")

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
	})
}

func TestCreateContactWithIdentities(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPost, "/api/contacts",
		`{"name": "María Pérez", "identities": [
			{"channel": "email", "identifier": " Maria@Example.COM "},
			{"channel": "phone", "identifier": "+184 467 235"}
		]}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	created := decodeBody[contactBody](t, recorder)
	stored := store.identities[created.ID]
	if len(stored) != 2 {
		t.Fatalf("stored identities = %d, want 2", len(stored))
	}
	if stored[0].Identifier != "maria@example.com" || stored[1].Identifier != "+184467235" {
		t.Errorf("stored identifiers = %q, %q, want the normalized pair", stored[0].Identifier, stored[1].Identifier)
	}
}

func TestCreateContactWithIdentitiesRejectsAClaimedIdentity(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	seedContacts(t, store, "María Pérez")
	maria := store.byName(t, "María Pérez")
	seedIdentity(t, store, maria.ID, "email", "maria@example.com")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPost, "/api/contacts",
		`{"name": "John Doe", "identities": [{"channel": "email", "identifier": "maria@example.com"}]}`)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusConflict, recorder.Body)
	}
	conflict := decodeBody[identityConflictBody](t, recorder)
	if conflict.Owner == nil || conflict.Owner.ID != maria.ID {
		t.Errorf("owner = %+v, want María Pérez", conflict.Owner)
	}
	if len(store.contacts) != 1 {
		t.Errorf("contacts stored = %d, want only the original 1", len(store.contacts))
	}
}

func TestCreateContactWithIdentitiesRejectsANonWritableChannel(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPost, "/api/contacts",
		`{"name": "John Doe", "identities": [{"channel": "whatsapp", "identifier": "184467235"}]}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body)
	}
	if len(store.contacts) != 0 {
		t.Errorf("contacts stored = %d, want 0", len(store.contacts))
	}
}

func TestGetContactReturnsAnEmptyIdentitiesArray(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	bruno, err := contact.New("Bruno")
	if err != nil {
		t.Fatalf("contact.New() error = %v, want nil", err)
	}
	if err := store.Create(t.Context(), bruno); err != nil {
		t.Fatalf("seeding Bruno: %v", err)
	}
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/"+bruno.ID.String(), "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"identities":[]`) {
		t.Errorf("body = %s, want an empty identities array rather than null", body)
	}
}

func TestGetContactReportsIdentityStoreFailure(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	ada, err := contact.New("Ada")
	if err != nil {
		t.Fatalf("contact.New() error = %v, want nil", err)
	}
	if err := store.Create(t.Context(), ada); err != nil {
		t.Fatalf("seeding Ada: %v", err)
	}
	store.identitiesErr = errors.New("boom")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodGet, "/api/contacts/"+ada.ID.String(), "")

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func seedOneContact(t *testing.T, store *fakeContactStore, name string) contact.Contact {
	t.Helper()
	c, err := contact.New(name)
	if err != nil {
		t.Fatalf("contact.New(%q) error = %v, want nil", name, err)
	}
	if err := store.Create(t.Context(), c); err != nil {
		t.Fatalf("seeding %q: %v", name, err)
	}
	return c
}

func TestRenameContactEndpoint(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	ada := seedOneContact(t, store, "34600111222")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/contacts/"+ada.ID.String(),
		`{"name":"  Ada Lovelace  "}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := decodeBody[contactBody](t, recorder)
	if body.ID != ada.ID || body.Name != "Ada Lovelace" {
		t.Errorf("body = %+v, want the contact renamed and trimmed", body)
	}
	if store.contacts[ada.ID].Name != "Ada Lovelace" {
		t.Errorf("stored name = %q, want %q", store.contacts[ada.ID].Name, "Ada Lovelace")
	}
}

func TestRenameContactEndpointTreatsOmittedNameAsNoOp(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	ada := seedOneContact(t, store, "Ada")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/contacts/"+ada.ID.String(), `{}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := decodeBody[contactBody](t, recorder)
	if body.Name != "Ada" {
		t.Errorf("body name = %q, want the unchanged %q", body.Name, "Ada")
	}
}

func TestRenameContactEndpointRejectsBadRequests(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	ada := seedOneContact(t, store, "Ada")

	tests := map[string]struct {
		target string
		body   string
		want   int
	}{
		"blank name": {
			target: "/api/contacts/" + ada.ID.String(),
			body:   `{"name":"   "}`,
			want:   http.StatusUnprocessableEntity,
		},
		"unknown id": {
			target: "/api/contacts/" + uuid.Must(uuid.NewV7()).String(),
			body:   `{"name":"Ada"}`,
			want:   http.StatusNotFound,
		},
		"omitted name unknown id": {
			target: "/api/contacts/" + uuid.Must(uuid.NewV7()).String(),
			body:   `{}`,
			want:   http.StatusNotFound,
		},
		"malformed id":   {target: "/api/contacts/not-a-uuid", body: `{"name":"Ada"}`, want: http.StatusBadRequest},
		"malformed json": {target: "/api/contacts/" + ada.ID.String(), body: `{`, want: http.StatusBadRequest},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			srv := authedContactServer(t, store, nil)

			recorder := doRequest(t, srv, http.MethodPatch, tc.target, tc.body)

			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.want)
			}
		})
	}
}

func TestRenameContactEndpointReportsStoreFailure(t *testing.T) {
	t.Parallel()

	store := newFakeContactStore()
	ada := seedOneContact(t, store, "Ada")
	store.renameErr = errors.New("boom")
	srv := authedContactServer(t, store, nil)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/contacts/"+ada.ID.String(), `{"name":"Ada L"}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestListContactsEndpointForwardsSearchQueries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		target     string
		wantQuery  string
		wantDigits string
	}{
		"phone with formatting": {
			target:     "/api/contacts?q=" + url.QueryEscape("+1 844 672"),
			wantQuery:  "+1 844 672",
			wantDigits: "1844672",
		},
		"plain name": {target: "/api/contacts?q=ana", wantQuery: "ana", wantDigits: ""},
		"no query":   {target: "/api/contacts", wantQuery: "", wantDigits: ""},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			store := newFakeContactStore()
			srv := authedContactServer(t, store, nil)

			recorder := doRequest(t, srv, http.MethodGet, tc.target, "")

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if store.lastQuery != tc.wantQuery {
				t.Errorf("store received query %q, want %q", store.lastQuery, tc.wantQuery)
			}
			if store.lastDigits != tc.wantDigits {
				t.Errorf("store received digits %q, want %q", store.lastDigits, tc.wantDigits)
			}
		})
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

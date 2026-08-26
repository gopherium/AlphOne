// SPDX-License-Identifier: Elastic-2.0

package contact_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
)

var _ contact.Store = (*fakeStore)(nil)

type identityKey struct {
	channel    contact.Channel
	identifier string
}

type fakeStore struct {
	contacts     map[uuid.UUID]contact.Contact
	identities   map[identityKey]contact.Identity
	lookupErr    error
	getErr       error
	createErr    error
	beforeCreate func()
}

// newFakeStore returns an empty in memory contact store.
func newFakeStore() *fakeStore {
	return &fakeStore{
		contacts:   map[uuid.UUID]contact.Contact{},
		identities: map[identityKey]contact.Identity{},
	}
}

// Get returns the stored contact the id names, or the configured failure.
func (f *fakeStore) Get(_ context.Context, id uuid.UUID) (contact.Contact, error) {
	if f.getErr != nil {
		return contact.Contact{}, f.getErr
	}
	c, ok := f.contacts[id]
	if !ok {
		return contact.Contact{}, contact.ErrNotFound
	}
	return c, nil
}

// LookupIdentity returns the stored identity for the channel and identifier, or the configured failure.
func (f *fakeStore) LookupIdentity(
	_ context.Context,
	channel contact.Channel,
	identifier string,
) (contact.Identity, error) {
	if f.lookupErr != nil {
		return contact.Identity{}, f.lookupErr
	}
	identity, ok := f.identities[identityKey{channel, identifier}]
	if !ok {
		return contact.Identity{}, contact.ErrIdentityNotFound
	}
	return identity, nil
}

// CreateContactWithIdentity stores one contact and one identity, refusing an identifier already claimed.
func (f *fakeStore) CreateContactWithIdentity(_ context.Context, c contact.Contact, identity contact.Identity) error {
	if f.beforeCreate != nil {
		f.beforeCreate()
	}
	if f.createErr != nil {
		return f.createErr
	}
	key := identityKey{identity.Channel, identity.Identifier}
	if _, ok := f.identities[key]; ok {
		return contact.ErrIdentityExists
	}
	f.contacts[c.ID] = c
	f.identities[key] = identity
	return nil
}

// CreateContactWithIdentities stores one contact and every given identity, refusing any identifier already claimed.
func (f *fakeStore) CreateContactWithIdentities(
	_ context.Context, c contact.Contact, identities []contact.Identity,
) error {
	if f.createErr != nil {
		return f.createErr
	}
	for _, identity := range identities {
		key := identityKey{identity.Channel, identity.Identifier}
		if claimed, ok := f.identities[key]; ok {
			return contact.IdentityExistsError{OwnerID: claimed.ContactID}
		}
	}
	f.contacts[c.ID] = c
	for _, identity := range identities {
		f.identities[identityKey{identity.Channel, identity.Identifier}] = identity
	}
	return nil
}

// seedContact stores a named contact with one channel identity and returns it.
func seedContact(
	t *testing.T,
	store *fakeStore,
	name string,
	channel contact.Channel,
	identifier string,
) contact.Contact {
	t.Helper()
	c, err := contact.New(name)
	if err != nil {
		t.Fatalf("New(%q) error = %v, want nil", name, err)
	}
	identity, err := contact.NewIdentity(c.ID, channel, identifier, "")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}
	if err := store.CreateContactWithIdentity(t.Context(), c, identity); err != nil {
		t.Fatalf("CreateContactWithIdentity() error = %v, want nil", err)
	}
	return c
}

func TestResolveValidatesInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		channel    contact.Channel
		identifier string
		wantErr    error
	}{
		"empty channel":    {channel: " \t ", identifier: "184467235@lid", wantErr: contact.ErrEmptyChannel},
		"empty identifier": {channel: "whatsapp", identifier: " \t ", wantErr: contact.ErrEmptyIdentifier},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			resolver := contact.NewResolver(newFakeStore())

			_, err := resolver.Resolve(t.Context(), tc.channel, tc.identifier, "María")

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Resolve() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestResolveReturnsExistingContact(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	maria := seedContact(t, store, "María Pérez", "whatsapp", "184467235@lid")
	resolver := contact.NewResolver(store)

	got, err := resolver.Resolve(t.Context(), "  WhatsApp ", "184467235@lid", "ignored")

	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got.ID != maria.ID {
		t.Errorf("Resolve().ID = %s, want existing contact %s", got.ID, maria.ID)
	}
	if len(store.contacts) != 1 {
		t.Errorf("store holds %d contacts, want 1", len(store.contacts))
	}
}

func TestResolveCreatesContactForUnknownIdentity(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	resolver := contact.NewResolver(store)

	got, err := resolver.Resolve(t.Context(), "whatsapp", "184467235@lid", " María Pérez ")

	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got.Name != "María Pérez" {
		t.Errorf("Resolve().Name = %q, want %q", got.Name, "María Pérez")
	}
	identity, ok := store.identities[identityKey{"whatsapp", "184467235@lid"}]
	if !ok {
		t.Fatal("identity was not stored")
	}
	if identity.ContactID != got.ID {
		t.Errorf("stored identity.ContactID = %s, want %s", identity.ContactID, got.ID)
	}

	again, err := resolver.Resolve(t.Context(), "whatsapp", "184467235@lid", "María Pérez")

	if err != nil {
		t.Fatalf("second Resolve() error = %v, want nil", err)
	}
	if again.ID != got.ID || len(store.contacts) != 1 {
		t.Errorf("second Resolve() ID = %s with %d contacts, want %s with 1", again.ID, len(store.contacts), got.ID)
	}
}

func TestResolveNamesContactAfterIdentifierWithoutDisplayName(t *testing.T) {
	t.Parallel()

	resolver := contact.NewResolver(newFakeStore())

	got, err := resolver.Resolve(t.Context(), "whatsapp", "184467235@lid", " \t ")

	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got.Name != "184467235@lid" {
		t.Errorf("Resolve().Name = %q, want %q", got.Name, "184467235@lid")
	}
}

func TestResolveReturnsRaceWinner(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	var winner contact.Contact
	store.beforeCreate = func() {
		store.beforeCreate = nil
		winner = seedContact(t, store, "María Pérez", "whatsapp", "184467235@lid")
	}
	resolver := contact.NewResolver(store)

	got, err := resolver.Resolve(t.Context(), "whatsapp", "184467235@lid", "Loser Draft")

	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got.ID != winner.ID {
		t.Errorf("Resolve().ID = %s, want race winner %s", got.ID, winner.ID)
	}
	if len(store.contacts) != 1 {
		t.Errorf("store holds %d contacts, want only the winner", len(store.contacts))
	}
}

func TestResolveReportsStoreFailures(t *testing.T) {
	t.Parallel()

	errStore := errors.New("store exploded")

	t.Run("lookup fails", func(t *testing.T) {
		t.Parallel()

		store := newFakeStore()
		store.lookupErr = errStore

		_, err := contact.NewResolver(store).Resolve(t.Context(), "whatsapp", "184467235@lid", "")

		if !errors.Is(err, errStore) {
			t.Fatalf("Resolve() error = %v, want %v", err, errStore)
		}
	})

	t.Run("owner fetch fails", func(t *testing.T) {
		t.Parallel()

		store := newFakeStore()
		seedContact(t, store, "María Pérez", "whatsapp", "184467235@lid")
		store.getErr = errStore

		_, err := contact.NewResolver(store).Resolve(t.Context(), "whatsapp", "184467235@lid", "")

		if !errors.Is(err, errStore) {
			t.Fatalf("Resolve() error = %v, want %v", err, errStore)
		}
	})

	t.Run("create fails", func(t *testing.T) {
		t.Parallel()

		store := newFakeStore()
		store.createErr = errStore

		_, err := contact.NewResolver(store).Resolve(t.Context(), "whatsapp", "184467235@lid", "")

		if !errors.Is(err, errStore) {
			t.Fatalf("Resolve() error = %v, want %v", err, errStore)
		}
	})
}

func TestResolveReportsIDGenerationFailure(t *testing.T) {
	t.Run("contact id", func(t *testing.T) {
		uuid.SetRand(&flakyReader{allowed: 0})
		defer uuid.SetRand(nil)

		_, err := contact.NewResolver(newFakeStore()).Resolve(t.Context(), "whatsapp", "184467235@lid", "")

		if !errors.Is(err, errEntropy) {
			t.Fatalf("Resolve() error = %v, want the entropy failure in its chain", err)
		}
	})

	t.Run("identity id", func(t *testing.T) {
		uuid.SetRand(&flakyReader{allowed: 1})
		defer uuid.SetRand(nil)

		_, err := contact.NewResolver(newFakeStore()).Resolve(t.Context(), "whatsapp", "184467235@lid", "")

		if !errors.Is(err, errEntropy) {
			t.Fatalf("Resolve() error = %v, want the entropy failure in its chain", err)
		}
	})
}

type flakyReader struct {
	allowed int
}

// Read fills the buffer for the allowed number of calls and then fails.
func (r *flakyReader) Read(p []byte) (int, error) {
	if r.allowed == 0 {
		return 0, errEntropy
	}
	r.allowed--
	for i := range p {
		p[i] = byte(i + 1)
	}
	return len(p), nil
}

type recordingEvents struct {
	names []event.Name
	data  []map[string]any
}

// Publish keeps the name and data of every published frame.
func (r *recordingEvents) Publish(_ context.Context, frame event.Frame, data map[string]any) {
	r.names = append(r.names, frame.Name)
	r.data = append(r.data, data)
}

func TestResolveAnnouncesAContactItCreates(t *testing.T) {
	t.Parallel()

	events := &recordingEvents{}
	resolver := contact.NewResolver(newFakeStore(), contact.WithEvents(events))

	created, err := resolver.Resolve(t.Context(), "whatsapp", "184467235@lid", "Maria Perez")

	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if len(events.names) != 1 || events.names[0] != event.ContactCreated {
		t.Fatalf("published %v, want one %q", events.names, event.ContactCreated)
	}
	if events.data[0]["id"] != created.ID.String() {
		t.Errorf("data.id = %v, want %q", events.data[0]["id"], created.ID)
	}
	if events.data[0]["name"] != "Maria Perez" {
		t.Errorf("data.name = %v, want the contact name", events.data[0]["name"])
	}
}

func TestResolveStaysQuietForAKnownContact(t *testing.T) {
	t.Parallel()

	events := &recordingEvents{}
	resolver := contact.NewResolver(newFakeStore(), contact.WithEvents(events))
	if _, err := resolver.Resolve(t.Context(), "whatsapp", "184467235@lid", "Maria Perez"); err != nil {
		t.Fatalf("first Resolve() error = %v, want nil", err)
	}
	events.names = nil

	if _, err := resolver.Resolve(t.Context(), "whatsapp", "184467235@lid", "Maria Perez"); err != nil {
		t.Fatalf("second Resolve() error = %v, want nil", err)
	}

	if len(events.names) != 0 {
		t.Errorf("published %v for a contact that already existed, want nothing", events.names)
	}
}

func TestResolveWorksWithoutAPublisher(t *testing.T) {
	t.Parallel()

	resolver := contact.NewResolver(newFakeStore())

	if _, err := resolver.Resolve(t.Context(), "whatsapp", "184467235@lid", "Maria Perez"); err != nil {
		t.Errorf("Resolve() error = %v, want nil without a publisher configured", err)
	}
}

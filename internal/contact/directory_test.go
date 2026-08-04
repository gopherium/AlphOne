// SPDX-License-Identifier: Elastic-2.0

package contact_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
)

var errDirectoryStore = errors.New("the contact store is unreachable")

func TestFindByIdentityReportsAKnownOwner(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	resolver := contact.NewResolver(store)
	owner := seedContact(t, store, "Maria Perez", "email", "maria@example.com")

	got, found, err := resolver.FindByIdentity(t.Context(), "email", "MARIA@example.com")

	if err != nil {
		t.Fatalf("FindByIdentity() error = %v, want nil", err)
	}
	if !found {
		t.Fatal("FindByIdentity() found = false, want the seeded owner")
	}
	if got.ID != owner.ID {
		t.Errorf("owner = %v, want %v", got.ID, owner.ID)
	}
}

func TestFindByIdentityReportsAnUnknownOwner(t *testing.T) {
	t.Parallel()

	resolver := contact.NewResolver(newFakeStore())

	got, found, err := resolver.FindByIdentity(t.Context(), "email", "nobody@example.com")

	if err != nil {
		t.Fatalf("FindByIdentity() error = %v, want nil", err)
	}
	if found {
		t.Errorf("FindByIdentity() found = true with %v, want false", got)
	}
}

func TestFindByIdentityValidatesItsInput(t *testing.T) {
	t.Parallel()

	resolver := contact.NewResolver(newFakeStore())

	if _, _, err := resolver.FindByIdentity(t.Context(), "", "maria@example.com"); err == nil {
		t.Error("FindByIdentity() with a blank channel error = nil, want one")
	}
	if _, _, err := resolver.FindByIdentity(t.Context(), "email", "  "); err == nil {
		t.Error("FindByIdentity() with a blank identifier error = nil, want one")
	}
}

func TestFindByIdentityReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.lookupErr = errDirectoryStore
	resolver := contact.NewResolver(store)

	if _, _, err := resolver.FindByIdentity(t.Context(), "email", "maria@example.com"); err == nil {
		t.Error("FindByIdentity() error = nil, want the store failure")
	}
}

func TestCreateWithIdentitiesReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.createErr = errDirectoryStore
	resolver := contact.NewResolver(store)

	_, _, err := resolver.CreateWithIdentities(t.Context(), "Maria Perez",
		[]contact.Address{{Channel: "email", Identifier: "maria@example.com"}})

	if err == nil {
		t.Error("CreateWithIdentities() error = nil, want the store failure")
	}
}

func TestCreateWithIdentitiesReportsAFailedOwnerLookup(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seedContact(t, store, "Maria Perez", "email", "maria@example.com")
	store.getErr = errDirectoryStore
	resolver := contact.NewResolver(store)

	_, _, err := resolver.CreateWithIdentities(t.Context(), "Maria P",
		[]contact.Address{{Channel: "email", Identifier: "maria@example.com"}})

	if err == nil {
		t.Error("CreateWithIdentities() error = nil, want the owner lookup failure")
	}
}

func TestCreateWithIdentitiesStoresEveryAddressNormalized(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	events := &recordingEvents{}
	resolver := contact.NewResolver(store, contact.WithEvents(events))
	addresses := []contact.Address{
		{Channel: "email", Identifier: "MARIA@Example.COM"},
		{Channel: "phone", Identifier: "+184 467 235"},
	}

	created, wasCreated, err := resolver.CreateWithIdentities(t.Context(), "Maria Perez", addresses)

	if err != nil {
		t.Fatalf("CreateWithIdentities() error = %v, want nil", err)
	}
	if !wasCreated {
		t.Fatal("CreateWithIdentities() created = false, want true")
	}
	if created.Name != "Maria Perez" {
		t.Errorf("name = %q, want Maria Perez", created.Name)
	}
	if _, ok := store.identities[identityKey{"email", "maria@example.com"}]; !ok {
		t.Error("the email identity was not stored lowercased")
	}
	if _, ok := store.identities[identityKey{"phone", "+184467235"}]; !ok {
		t.Error("the phone identity was not stored as digits behind a plus")
	}
	if !slices.Contains(events.names, event.ContactCreated) {
		t.Errorf("published %v, want %q", events.names, event.ContactCreated)
	}
}

func TestCreateWithIdentitiesReportsTheClaimingOwner(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	events := &recordingEvents{}
	resolver := contact.NewResolver(store, contact.WithEvents(events))
	owner := seedContact(t, store, "Maria Perez", "email", "maria@example.com")

	got, wasCreated, err := resolver.CreateWithIdentities(t.Context(), "Maria P",
		[]contact.Address{{Channel: "email", Identifier: "maria@example.com"}})

	if err != nil {
		t.Fatalf("CreateWithIdentities() error = %v, want nil", err)
	}
	if wasCreated {
		t.Error("CreateWithIdentities() created = true, want false for a claimed identity")
	}
	if got.ID != owner.ID {
		t.Errorf("owner = %v, want the claiming contact %v", got.ID, owner.ID)
	}
	if slices.Contains(events.names, event.ContactCreated) {
		t.Errorf("published %v for a contact that was not created", events.names)
	}
}

func TestCreateWithIdentitiesValidatesItsInput(t *testing.T) {
	t.Parallel()

	resolver := contact.NewResolver(newFakeStore())

	if _, _, err := resolver.CreateWithIdentities(t.Context(), "  ", nil); err == nil {
		t.Error("CreateWithIdentities() with a blank name error = nil, want one")
	}
	_, _, err := resolver.CreateWithIdentities(t.Context(), "Maria Perez",
		[]contact.Address{{Channel: "email", Identifier: "  "}})
	if err == nil {
		t.Error("CreateWithIdentities() with a blank identifier error = nil, want one")
	}
}

// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/task"
)

// The fakes double the stores the graph resolves from.
var (
	_ graphres.ContactStore = (*postgres.ContactStore)(nil)
	_ graphres.ContactStore = (*fakeContactStore)(nil)
	_ graphres.TaskStore    = (*postgres.TaskStore)(nil)
	_ graphres.TaskStore    = (*fakeTaskStore)(nil)
)

type fakeContactStore struct {
	contacts   map[uuid.UUID]contact.Contact
	identities map[uuid.UUID][]contact.Identity
}

// newFakeContactStore returns an empty in memory contact store.
func newFakeContactStore() *fakeContactStore {
	return &fakeContactStore{
		contacts:   map[uuid.UUID]contact.Contact{},
		identities: map[uuid.UUID][]contact.Identity{},
	}
}

// ListContactIdentities returns the identities stored under the named contact.
func (f *fakeContactStore) ListContactIdentities(
	_ context.Context, contactID uuid.UUID,
) ([]contact.Identity, error) {
	return f.identities[contactID], nil
}

// ListByIDs returns the stored contacts the given identifiers name.
func (f *fakeContactStore) ListByIDs(_ context.Context, ids []uuid.UUID) ([]contact.Contact, error) {
	var found []contact.Contact
	for _, id := range ids {
		if c, ok := f.contacts[id]; ok {
			found = append(found, c)
		}
	}
	return found, nil
}

// identityOwner returns the contact holding the given channel identity and whether one holds it.
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

// AddIdentity stores one identity under an existing contact and refuses an identity another contact holds.
func (f *fakeContactStore) AddIdentity(_ context.Context, identity contact.Identity) error {
	if _, ok := f.contacts[identity.ContactID]; !ok {
		return contact.ErrNotFound
	}
	if ownerID, claimed := f.identityOwner(identity.Channel, identity.Identifier); claimed {
		return contact.IdentityExistsError{OwnerID: ownerID}
	}
	f.identities[identity.ContactID] = append(f.identities[identity.ContactID], identity)
	return nil
}

// DeleteIdentity removes the named identity from the named contact.
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

// CreateContactWithIdentities stores one contact with its identities and refuses an identity already held.
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

// RenameContact gives the named contact a new name and returns it.
func (f *fakeContactStore) RenameContact(
	_ context.Context, id uuid.UUID, name string,
) (contact.Contact, error) {
	c, ok := f.contacts[id]
	if !ok {
		return contact.Contact{}, contact.ErrNotFound
	}
	c.Name = name
	f.contacts[id] = c
	return c, nil
}

// Create stores one contact.
func (f *fakeContactStore) Create(_ context.Context, c contact.Contact) error {
	f.contacts[c.ID] = c
	return nil
}

// Get returns the stored contact the identifier names.
func (f *fakeContactStore) Get(_ context.Context, id uuid.UUID) (contact.Contact, error) {
	c, ok := f.contacts[id]
	if !ok {
		return contact.Contact{}, contact.ErrNotFound
	}
	return c, nil
}

// ListContacts returns one page of stored contacts ordered by name then identifier.
func (f *fakeContactStore) ListContacts(
	_ context.Context, _, _ string, afterName string, afterID uuid.UUID, limit int,
) ([]contact.Contact, error) {
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

type fakeTaskStore struct {
	tasks map[uuid.UUID]task.Task
}

// newFakeTaskStore returns an empty in memory task store.
func newFakeTaskStore() *fakeTaskStore {
	return &fakeTaskStore{tasks: map[uuid.UUID]task.Task{}}
}

// ListForDay returns no tasks.
func (f *fakeTaskStore) ListForDay(
	_ context.Context, _ uuid.UUID, _ time.Time, _ string, _ task.Page,
) ([]task.Task, error) {
	return nil, nil
}

// ListDueBefore returns no tasks.
func (f *fakeTaskStore) ListDueBefore(
	_ context.Context, _ uuid.UUID, _ time.Time, _ string, _ task.Page,
) ([]task.Task, error) {
	return nil, nil
}

// ListForContact returns no tasks.
func (f *fakeTaskStore) ListForContact(
	_ context.Context, _ uuid.UUID, _ string, _ task.Page,
) ([]task.Task, error) {
	return nil, nil
}

// Create stores one task and reports whether it is new, returning the task already stored under the same origin.
func (f *fakeTaskStore) Create(_ context.Context, t task.Task) (task.Task, bool, error) {
	if stored, ok := f.byOrigin(t); ok {
		return stored, false, nil
	}
	f.tasks[t.ID] = t
	return t, true, nil
}

// byOrigin returns the assignee's task already stored under an origin event.
func (f *fakeTaskStore) byOrigin(t task.Task) (task.Task, bool) {
	if t.Origin.EventID == uuid.Nil {
		return task.Task{}, false
	}
	for _, stored := range f.tasks {
		if stored.Origin == t.Origin && stored.AssigneeID == t.AssigneeID {
			return stored, true
		}
	}
	return task.Task{}, false
}

// Update replaces the stored task with the given one.
func (f *fakeTaskStore) Update(_ context.Context, t task.Task) (task.Task, error) {
	if _, ok := f.tasks[t.ID]; !ok {
		return task.Task{}, task.ErrNotFound
	}
	f.tasks[t.ID] = t
	return t, nil
}

// Get returns the stored task the identifier names.
func (f *fakeTaskStore) Get(_ context.Context, id uuid.UUID) (task.Task, error) {
	stored, ok := f.tasks[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	return stored, nil
}

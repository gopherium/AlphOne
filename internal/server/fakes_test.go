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

// The fakes double the stores the graph resolves from, shared by the
// remaining server tests.
var (
	_ graphres.ContactStore = (*postgres.ContactStore)(nil)
	_ graphres.ContactStore = (*fakeContactStore)(nil)
	_ graphres.TaskStore    = (*postgres.TaskStore)(nil)
	_ graphres.TaskStore    = (*fakeTaskStore)(nil)
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

func (f *fakeContactStore) ListByIDs(_ context.Context, ids []uuid.UUID) ([]contact.Contact, error) {
	var found []contact.Contact
	for _, id := range ids {
		if c, ok := f.contacts[id]; ok {
			found = append(found, c)
		}
	}
	return found, nil
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

type taskListCall struct {
	mode       string
	assigneeID uuid.UUID
	contactID  uuid.UUID
	on         time.Time
	status     string
	page       task.Page
}

type fakeTaskStore struct {
	tasks     map[uuid.UUID]task.Task
	listed    []task.Task
	lastList  taskListCall
	createErr error
	getErr    error
	listErr   error
	updateErr error
}

func newFakeTaskStore() *fakeTaskStore {
	return &fakeTaskStore{tasks: map[uuid.UUID]task.Task{}}
}

func (f *fakeTaskStore) ListForDay(
	_ context.Context, assigneeID uuid.UUID, dueOn time.Time, status string, page task.Page,
) ([]task.Task, error) {
	f.lastList = taskListCall{
		mode: "day", assigneeID: assigneeID, on: dueOn, status: status, page: page,
	}
	return f.listPage(page)
}

func (f *fakeTaskStore) ListDueBefore(
	_ context.Context, assigneeID uuid.UUID, dueBefore time.Time, status string, page task.Page,
) ([]task.Task, error) {
	f.lastList = taskListCall{
		mode: "before", assigneeID: assigneeID, on: dueBefore, status: status, page: page,
	}
	return f.listPage(page)
}

func (f *fakeTaskStore) ListForContact(
	_ context.Context, contactID uuid.UUID, status string, page task.Page,
) ([]task.Task, error) {
	f.lastList = taskListCall{mode: "contact", contactID: contactID, status: status, page: page}
	return f.listPage(page)
}

func (f *fakeTaskStore) listPage(page task.Page) ([]task.Task, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.listed) > page.Limit {
		return f.listed[:page.Limit], nil
	}
	return f.listed, nil
}

func (f *fakeTaskStore) Create(_ context.Context, t task.Task) (task.Task, bool, error) {
	if f.createErr != nil {
		return task.Task{}, false, f.createErr
	}
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

func (f *fakeTaskStore) Update(_ context.Context, t task.Task) (task.Task, error) {
	if f.updateErr != nil {
		return task.Task{}, f.updateErr
	}
	if _, ok := f.tasks[t.ID]; !ok {
		return task.Task{}, task.ErrNotFound
	}
	f.tasks[t.ID] = t
	return t, nil
}

func (f *fakeTaskStore) Get(_ context.Context, id uuid.UUID) (task.Task, error) {
	if f.getErr != nil {
		return task.Task{}, f.getErr
	}
	stored, ok := f.tasks[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	return stored, nil
}

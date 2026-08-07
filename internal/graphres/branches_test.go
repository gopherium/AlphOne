// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/sdk"
)

func TestFromPluginsRequiresEveryGraphPlugin(t *testing.T) {
	t.Parallel()

	_, err := graphroot.FromPlugins(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "importer") {
		t.Errorf("FromPlugins(no plugins) error = %v, want the importer complaint", err)
	}

	deps := sdk.Deps{DatabaseURL: "postgres://graph:graph@localhost:1/graph"}
	importerPlugin, err := importer.Register(deps)
	if err != nil {
		t.Fatalf("importer.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = importerPlugin.Stop(context.Background()) })

	_, err = graphroot.FromPlugins(nil, []sdk.Plugin{importerPlugin})
	if err == nil || !strings.Contains(err.Error(), "whatsapp") {
		t.Errorf("FromPlugins(importer only) error = %v, want the whatsapp complaint", err)
	}
}

// scopedContext returns a context carrying a fresh request scope.
func scopedContext() context.Context {
	return sdk.WithRequestScope(context.Background(), sdk.NewRequestScope())
}

// newStubResolver returns a resolver over the given stub stores without events.
func newStubResolver(contacts *stubContactStore, tasks *stubTaskStore) *graphres.Resolver {
	return &graphres.Resolver{Version: "9.9.9", Contacts: contacts, Tasks: tasks}
}

func TestContactsQueryValidatesArguments(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, newStubResolver(&stubContactStore{}, &stubTaskStore{}), uuid.Must(uuid.NewV7()))

	tooSmall, err := client.RawPost(`{ contacts(first: 0) { pageInfo { hasNextPage } } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, tooSmall.Errors); got != "VALIDATION" {
		t.Errorf("first zero code = %q, want VALIDATION", got)
	}

	var digits struct {
		Contacts connectionResult `json:"contacts"`
	}
	client.MustPost(`{ contacts(q: "184467235") { edges { node { id } } pageInfo { hasNextPage } } }`, &digits)
	if len(digits.Contacts.Edges) != 0 {
		t.Errorf("digit search edges = %d, want none from the empty stub", len(digits.Contacts.Edges))
	}
}

func TestContactFieldResolverBranches(t *testing.T) {
	t.Parallel()

	maria := contact.Contact{ID: uuid.Must(uuid.NewV7()), Name: "Maria Perez", CreatedAt: time.Now()}
	resolver := newStubResolver(
		&stubContactStore{contacts: map[uuid.UUID]contact.Contact{maria.ID: maria}}, &stubTaskStore{},
	)

	createdAt, err := resolver.ContactResolvers().CreatedAt(scopedContext(), &model.Contact{ID: maria.ID})
	if err != nil || createdAt == nil {
		t.Errorf("CreatedAt(known) = (%v, %v), want the loaded time", createdAt, err)
	}

	missingID := uuid.Must(uuid.NewV7())
	_, err = resolver.ContactResolvers().CreatedAt(scopedContext(), &model.Contact{ID: missingID})
	if !errors.Is(err, contact.ErrNotFound) {
		t.Errorf("CreatedAt(missing) error = %v, want ErrNotFound", err)
	}

	if _, err := resolver.ContactResolvers().CreatedAt(context.Background(), &model.Contact{ID: maria.ID}); err == nil {
		t.Error("CreatedAt() without a request scope error = nil, want error")
	}

	failing := newStubResolver(&stubContactStore{listByIDsErr: errors.New("contact store down")}, &stubTaskStore{})
	if _, err := failing.ContactResolvers().CreatedAt(scopedContext(), &model.Contact{ID: maria.ID}); err == nil {
		t.Error("CreatedAt() on a failing store error = nil, want error")
	}

	badIdentities := newStubResolver(&stubContactStore{identitiesErr: errors.New("identity store down")}, &stubTaskStore{})
	if _, err := badIdentities.ContactResolvers().Identities(scopedContext(), &model.Contact{ID: maria.ID}); err == nil {
		t.Error("Identities() on a failing store error = nil, want error")
	}

	if _, err := resolver.QueryResolvers().Contact(context.Background(), maria.ID); err != nil {
		t.Errorf("Contact() without a request scope error = %v, want the prime tolerated", err)
	}
}

func TestTasksQueryArgumentBranches(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, newStubResolver(&stubContactStore{}, &stubTaskStore{}), uuid.Must(uuid.NewV7()))

	rejected := map[string]string{
		"unknown status":   `{ tasks(date: "2026-08-06", status: "bogus") { pageInfo { hasNextPage } } }`,
		"first too small":  `{ tasks(date: "2026-08-06", first: 0) { pageInfo { hasNextPage } } }`,
		"malformed cursor": `{ tasks(date: "2026-08-06", after: "%%%") { pageInfo { hasNextPage } } }`,
	}
	for name, query := range rejected {
		response, err := client.RawPost(query)
		if err != nil {
			t.Fatalf("%s RawPost() error = %v, want nil", name, err)
		}
		if got := firstErrorCode(t, response.Errors); got != "VALIDATION" {
			t.Errorf("%s code = %q, want VALIDATION", name, got)
		}
	}

	accepted := map[string]string{
		"explicit status":   `{ tasks(date: "2026-08-06", status: "done") { pageInfo { hasNextPage } } }`,
		"due before filter": `{ tasks(dueBefore: "2026-08-06") { pageInfo { hasNextPage } } }`,
		"contact filter":    fmt.Sprintf(`{ tasks(contactId: %q) { pageInfo { hasNextPage } } }`, uuid.Must(uuid.NewV7())),
	}
	for name, query := range accepted {
		var page struct {
			Tasks connectionResult `json:"tasks"`
		}
		client.MustPost(query, &page)
		if len(page.Tasks.Edges) != 0 {
			t.Errorf("%s edges = %d, want none from the empty stub", name, len(page.Tasks.Edges))
		}
	}
}

func TestTaskQueryResolvesByID(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	stored := task.Task{ID: uuid.Must(uuid.NewV7()), Title: "Call the supplier", DueOn: day}
	client := newGraphClient(t, newStubResolver(&stubContactStore{}, &stubTaskStore{tasks: []task.Task{stored}}),
		uuid.Must(uuid.NewV7()))

	var found struct {
		Task struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"task"`
	}
	client.MustPost(fmt.Sprintf(`{ task(id: %q) { id title } }`, stored.ID), &found)
	if found.Task.Title != "Call the supplier" {
		t.Errorf("task = %+v, want the stored task", found.Task)
	}

	missing, err := client.RawPost(fmt.Sprintf(`{ task(id: %q) { id } }`, uuid.Must(uuid.NewV7())))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, missing.Errors); got != "NOT_FOUND" {
		t.Errorf("missing task code = %q, want NOT_FOUND", got)
	}
}

func TestTaskContactFieldHandlesAbsentAndFailingContacts(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	unlinked := task.Task{ID: uuid.Must(uuid.NewV7()), Title: "Order supplies", DueOn: day}
	client := newGraphClient(t, newStubResolver(&stubContactStore{}, &stubTaskStore{tasks: []task.Task{unlinked}}),
		uuid.Must(uuid.NewV7()))

	var page struct {
		Tasks struct {
			Edges []struct {
				Node struct {
					Contact *struct {
						ID string `json:"id"`
					} `json:"contact"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"tasks"`
	}
	client.MustPost(`{ tasks(date: "2026-08-06") { edges { node { contact { id } } } } }`, &page)
	if len(page.Tasks.Edges) != 1 || page.Tasks.Edges[0].Node.Contact != nil {
		t.Errorf("edges = %+v, want one task without a contact", page.Tasks.Edges)
	}

	linked := task.Task{
		ID: uuid.Must(uuid.NewV7()), Title: "Order supplies", DueOn: day, ContactID: uuid.Must(uuid.NewV7()),
	}
	failingClient := newGraphClient(t, newStubResolver(
		&stubContactStore{listByIDsErr: errors.New("contact store down")},
		&stubTaskStore{tasks: []task.Task{linked}},
	), uuid.Must(uuid.NewV7()))
	response, err := failingClient.RawPost(`{ tasks(date: "2026-08-06") { edges { node { contact { id } } } } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
		t.Errorf("failing contact load code = %q, want INTERNAL", got)
	}
}

func TestTaskMutationsSurfaceStoreFailures(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	stored := task.Task{ID: uuid.Must(uuid.NewV7()), Title: "Call the supplier", DueOn: day, Status: task.StatusOpen}
	stub := &stubTaskStore{
		tasks:     []task.Task{stored},
		createErr: errors.New("task store down"),
		updateErr: errors.New("task store down"),
	}
	client := newGraphClient(t, newStubResolver(&stubContactStore{}, stub), uuid.Must(uuid.NewV7()))

	create, err := client.RawPost(
		`mutation { createTask(input: {title: "Order supplies", dueOn: "2026-08-06"}) { task { id } } }`,
	)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, create.Errors); got != "INTERNAL" {
		t.Errorf("create code = %q, want INTERNAL", got)
	}

	update, err := client.RawPost(fmt.Sprintf(
		`mutation { updateTask(id: %q, input: {status: "done"}) { id } }`, stored.ID,
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, update.Errors); got != "INTERNAL" {
		t.Errorf("update code = %q, want INTERNAL", got)
	}
}

func TestUpdateTaskWithoutChangesReturnsTheStoredTask(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	stored := task.Task{ID: uuid.Must(uuid.NewV7()), Title: "Call the supplier", DueOn: day, Status: task.StatusOpen}
	stub := &stubTaskStore{tasks: []task.Task{stored}, updateErr: errors.New("update must not run")}
	client := newGraphClient(t, newStubResolver(&stubContactStore{}, stub), uuid.Must(uuid.NewV7()))

	var response struct {
		UpdateTask struct {
			Title string `json:"title"`
		} `json:"updateTask"`
	}
	client.MustPost(fmt.Sprintf(`mutation { updateTask(id: %q, input: {}) { title } }`, stored.ID), &response)

	if response.UpdateTask.Title != "Call the supplier" {
		t.Errorf("updateTask = %+v, want the stored task untouched", response.UpdateTask)
	}
}

func TestCreateContactStoresIdentitiesTransactionally(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, newStubResolver(&stubContactStore{}, &stubTaskStore{}), uuid.Must(uuid.NewV7()))

	var response struct {
		CreateContact struct {
			Name string `json:"name"`
		} `json:"createContact"`
	}
	client.MustPost(
		`mutation { createContact(name: "Maria Perez", identities: [
			{channel: "email", identifier: "maria@example.com"}
		]) { name } }`,
		&response,
	)

	if response.CreateContact.Name != "Maria Perez" {
		t.Errorf("createContact = %+v, want Maria Perez stored with her identity", response.CreateContact)
	}

	var plain struct {
		CreateContact struct {
			Name string `json:"name"`
		} `json:"createContact"`
	}
	client.MustPost(`mutation { createContact(name: "Noah Chen") { name } }`, &plain)
	if plain.CreateContact.Name != "Noah Chen" {
		t.Errorf("createContact = %+v, want the identity free contact stored", plain.CreateContact)
	}
}

func TestContactMutationsSurfaceStoreFailures(t *testing.T) {
	t.Parallel()

	stub := &stubContactStore{addIdentityErr: errors.New("identity store down")}
	client := newGraphClient(t, newStubResolver(stub, &stubTaskStore{}), uuid.Must(uuid.NewV7()))
	contactID := uuid.Must(uuid.NewV7())

	rename, err := client.RawPost(fmt.Sprintf(
		`mutation { renameContact(id: %q, name: "New Name") { name } }`, contactID,
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, rename.Errors); got != "NOT_FOUND" {
		t.Errorf("rename code = %q, want NOT_FOUND", got)
	}

	emptyName, err := client.RawPost(fmt.Sprintf(`mutation { renameContact(id: %q, name: "") { name } }`, contactID))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, emptyName.Errors); got != "VALIDATION" {
		t.Errorf("empty rename code = %q, want VALIDATION", got)
	}

	unwritable, err := client.RawPost(fmt.Sprintf(
		`mutation { addContactIdentity(contactId: %q, identity: {channel: "whatsapp", identifier: "184467235@lid"}) { id } }`,
		contactID,
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, unwritable.Errors); got != "VALIDATION" {
		t.Errorf("unwritable channel code = %q, want VALIDATION", got)
	}

	failing, err := client.RawPost(fmt.Sprintf(
		`mutation { addContactIdentity(contactId: %q, identity: {
			channel: "email", identifier: "maria@example.com"
		}) { id } }`,
		contactID,
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, failing.Errors); got != "INTERNAL" {
		t.Errorf("failing identity store code = %q, want INTERNAL", got)
	}

	tasksErr := &stubTaskStore{listForContactErr: errors.New("task store down")}
	tasksClient := newGraphClient(t, newStubResolver(&stubContactStore{
		contacts: map[uuid.UUID]contact.Contact{contactID: {ID: contactID, Name: "Maria Perez"}},
	}, tasksErr), uuid.Must(uuid.NewV7()))
	contactTasks, err := tasksClient.RawPost(fmt.Sprintf(
		`{ contact(id: %q) { tasks { pageInfo { hasNextPage } } } }`, contactID,
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, contactTasks.Errors); got != "INTERNAL" {
		t.Errorf("contact tasks code = %q, want INTERNAL", got)
	}

	badArgs, err := tasksClient.RawPost(fmt.Sprintf(
		`{ contact(id: %q) { tasks(first: 0) { pageInfo { hasNextPage } } } }`, contactID,
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, badArgs.Errors); got != "VALIDATION" {
		t.Errorf("contact tasks first zero code = %q, want VALIDATION", got)
	}
}

// failingWebhookStore rejects every webhook operation with its error.
type failingWebhookStore struct {
	err error
}

func (s failingWebhookStore) CreateSubscription(context.Context, webhook.Subscription) error {
	return s.err
}

func (s failingWebhookStore) ListSubscriptionsForUser(context.Context, uuid.UUID) ([]webhook.Subscription, error) {
	return nil, s.err
}

func (s failingWebhookStore) DeleteSubscription(context.Context, uuid.UUID, uuid.UUID) error {
	return s.err
}

func TestWebhookResolversSurfaceStoreFailures(t *testing.T) {
	t.Parallel()

	resolver := newStubResolver(&stubContactStore{}, &stubTaskStore{})
	resolver.Webhooks = failingWebhookStore{err: errors.New("webhook store down")}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	operations := map[string]string{
		"listing":  `{ webhooks { id } }`,
		"creation": `mutation { createWebhook(url: "https://example.com/hook", events: ["task.created"]) { secret } }`,
		"deletion": fmt.Sprintf(`mutation { deleteWebhook(id: %q) }`, uuid.Must(uuid.NewV7())),
	}
	for name, query := range operations {
		response, err := client.RawPost(query)
		if err != nil {
			t.Fatalf("%s RawPost() error = %v, want nil", name, err)
		}
		if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
			t.Errorf("%s code = %q, want INTERNAL", name, got)
		}
	}
}

// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/credential"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
)

// recordingPublisher captures every published event in order.
type recordingPublisher struct {
	names    []event.Name
	payloads []map[string]any
}

// Publish records the event.
func (p *recordingPublisher) Publish(_ context.Context, name event.Name, data map[string]any) {
	p.names = append(p.names, name)
	p.payloads = append(p.payloads, data)
}

// countOf reports how many times name was published.
func (p *recordingPublisher) countOf(name event.Name) int {
	count := 0
	for _, published := range p.names {
		if published == name {
			count++
		}
	}
	return count
}

// mutationHarness bundles the DB backed resolver and its doubles.
type mutationHarness struct {
	resolver *graphres.Resolver
	contacts *postgres.ContactStore
	events   *recordingPublisher
	assignee uuid.UUID
	client   *gqlclient.Client
}

// newMutationHarness builds a DB backed resolver with a recording publisher.
func newMutationHarness(t *testing.T) mutationHarness {
	t.Helper()
	pool := newTestPool(t)
	contacts := postgres.NewContactStore(pool)
	events := &recordingPublisher{}
	resolver := &graphres.Resolver{
		Version:  "9.9.9",
		Contacts: contacts,
		Tasks:    postgres.NewTaskStore(pool),
		Webhooks: postgres.NewWebhookStore(pool),
		Events:   events,
	}
	assignee := uuid.Must(uuid.NewV7())
	return mutationHarness{
		resolver: resolver,
		contacts: contacts,
		events:   events,
		assignee: assignee,
		client:   newGraphClient(t, resolver, assignee),
	}
}

// originClient returns a client whose requests carry a token attribution.
func (h mutationHarness) originClient(t *testing.T, tokenName string) *gqlclient.Client {
	t.Helper()
	return newDecoratedGraphClient(t, h.resolver, func(ctx context.Context) context.Context {
		return credential.WithTokenOrigin(authkitIdentity(ctx, h.assignee), tokenName)
	})
}

// firstErrorExtensions returns the first error's extensions of a raw response.
func firstErrorExtensions(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var parsed []struct {
		Extensions map[string]any `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed) == 0 {
		t.Fatalf("no errors in response: %s (%v)", raw, err)
	}
	return parsed[0].Extensions
}

type createdContact struct {
	CreateContact struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		CreatedAt  string `json:"createdAt"`
		Identities []struct {
			Channel    string `json:"channel"`
			Identifier string `json:"identifier"`
		} `json:"identities"`
	} `json:"createContact"`
}

func TestCreateContactStoresIdentitiesAndPublishes(t *testing.T) {
	t.Parallel()

	h := newMutationHarness(t)

	var created createdContact
	h.client.MustPost(`mutation { createContact(
		name: "Maria Perez",
		identities: [{channel: "phone", identifier: "184467235", displayName: "Maria"}]
	) { id name createdAt identities { channel identifier } } }`, &created)

	if created.CreateContact.Name != "Maria Perez" {
		t.Errorf("name = %q, want Maria Perez", created.CreateContact.Name)
	}
	if created.CreateContact.CreatedAt == "" {
		t.Error("createdAt is empty, want the primed creation time")
	}
	if len(created.CreateContact.Identities) != 1 || created.CreateContact.Identities[0].Identifier != "184467235" {
		t.Errorf("identities = %+v, want the phone identity", created.CreateContact.Identities)
	}
	wantPayload := map[string]any{"id": created.CreateContact.ID, "name": "Maria Perez"}
	if len(h.events.names) != 1 || h.events.names[0] != event.ContactCreated {
		t.Fatalf("published = %v, want exactly one contact.created", h.events.names)
	}
	if diff := cmp.Diff(wantPayload, h.events.payloads[0]); diff != "" {
		t.Errorf("contact.created payload mismatch (-want +got):\n%s", diff)
	}
}

func TestAddContactIdentityNamesTheOwnerOfAClaimedIdentity(t *testing.T) {
	t.Parallel()

	h := newMutationHarness(t)
	var owner createdContact
	h.client.MustPost(`mutation { createContact(
		name: "Maria Perez", identities: [{channel: "phone", identifier: "184467235"}]
	) { id } }`, &owner)
	var other createdContact
	h.client.MustPost(`mutation { createContact(name: "John Doe") { id } }`, &other)

	conflict, err := h.client.RawPost(fmt.Sprintf(`mutation { addContactIdentity(
		contactId: %q, identity: {channel: "phone", identifier: "184467235"}
	) { id } }`, other.CreateContact.ID))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	extensions := firstErrorExtensions(t, conflict.Errors)
	if extensions["code"] != "CONFLICT" {
		t.Errorf("code = %v, want CONFLICT", extensions["code"])
	}
	if extensions["ownerContactId"] != owner.CreateContact.ID {
		t.Errorf("ownerContactId = %v, want %s", extensions["ownerContactId"], owner.CreateContact.ID)
	}
	if extensions["ownerName"] != "Maria Perez" {
		t.Errorf("ownerName = %v, want Maria Perez", extensions["ownerName"])
	}
}

func TestCreateContactRejectsConflictsAndBadInput(t *testing.T) {
	t.Parallel()

	h := newMutationHarness(t)
	var first createdContact
	h.client.MustPost(`mutation { createContact(
		name: "Maria Perez", identities: [{channel: "phone", identifier: "184467235"}]
	) { id } }`, &first)

	conflict, err := h.client.RawPost(`mutation { createContact(
		name: "John Doe", identities: [{channel: "phone", identifier: "184467235"}]
	) { id } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	extensions := firstErrorExtensions(t, conflict.Errors)
	if extensions["code"] != "CONFLICT" {
		t.Errorf("code = %v, want CONFLICT", extensions["code"])
	}
	if extensions["ownerContactId"] != first.CreateContact.ID {
		t.Errorf("ownerContactId = %v, want %s", extensions["ownerContactId"], first.CreateContact.ID)
	}

	blank, err := h.client.RawPost(`mutation { createContact(name: "") { id } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, blank.Errors); got != "VALIDATION" {
		t.Errorf("blank name code = %q, want VALIDATION", got)
	}

	unwritable, err := h.client.RawPost(`mutation { createContact(
		name: "John Doe", identities: [{channel: "whatsapp", identifier: "184467235@lid"}]
	) { id } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, unwritable.Errors); got != "VALIDATION" {
		t.Errorf("unwritable channel code = %q, want VALIDATION", got)
	}
}

func TestRenameContact(t *testing.T) {
	t.Parallel()

	h := newMutationHarness(t)
	maria := mustSeedContact(t, h.contacts, "Maria Perez")

	var renamed struct {
		RenameContact struct {
			Name string `json:"name"`
		} `json:"renameContact"`
	}
	h.client.MustPost(fmt.Sprintf(
		`mutation { renameContact(id: %q, name: "Maria Perez Garcia") { name } }`, maria.ID,
	), &renamed)

	if renamed.RenameContact.Name != "Maria Perez Garcia" {
		t.Errorf("name = %q, want Maria Perez Garcia", renamed.RenameContact.Name)
	}
	if len(h.events.names) != 0 {
		t.Errorf("published = %v, want nothing for a rename", h.events.names)
	}

	missing, err := h.client.RawPost(fmt.Sprintf(
		`mutation { renameContact(id: %q, name: "Nobody") { name } }`, uuid.Must(uuid.NewV7()),
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, missing.Errors); got != "NOT_FOUND" {
		t.Errorf("missing code = %q, want NOT_FOUND", got)
	}
}

func TestIdentityAddAndDelete(t *testing.T) {
	t.Parallel()

	h := newMutationHarness(t)
	maria := mustSeedContact(t, h.contacts, "Maria Perez")

	var added struct {
		AddContactIdentity struct {
			ID         string `json:"id"`
			Identifier string `json:"identifier"`
		} `json:"addContactIdentity"`
	}
	h.client.MustPost(fmt.Sprintf(`mutation { addContactIdentity(
		contactId: %q, identity: {channel: "email", identifier: "maria@example.com"}
	) { id identifier } }`, maria.ID), &added)

	if added.AddContactIdentity.Identifier != "maria@example.com" {
		t.Errorf("identifier = %q, want maria@example.com", added.AddContactIdentity.Identifier)
	}

	var deleted struct {
		DeleteContactIdentity bool `json:"deleteContactIdentity"`
	}
	h.client.MustPost(fmt.Sprintf(
		`mutation { deleteContactIdentity(contactId: %q, identityId: %q) }`,
		maria.ID, added.AddContactIdentity.ID,
	), &deleted)

	if !deleted.DeleteContactIdentity {
		t.Error("deleteContactIdentity = false, want true")
	}

	gone, err := h.client.RawPost(fmt.Sprintf(
		`mutation { deleteContactIdentity(contactId: %q, identityId: %q) }`,
		maria.ID, added.AddContactIdentity.ID,
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, gone.Errors); got != "NOT_FOUND" {
		t.Errorf("deleted twice code = %q, want NOT_FOUND", got)
	}
}

type createdTask struct {
	CreateTask struct {
		Task struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			DueOn  string `json:"dueOn"`
		} `json:"task"`
		Replay bool `json:"replay"`
	} `json:"createTask"`
}

func TestCreateTaskPublishesThePinnedPayload(t *testing.T) {
	t.Parallel()

	h := newMutationHarness(t)

	var created createdTask
	h.client.MustPost(`mutation { createTask(input: {
		title: "Call Maria", dueOn: "2026-08-07", priority: 2
	}) { task { id status dueOn } replay } }`, &created)

	if created.CreateTask.Replay {
		t.Error("replay = true, want false on a first create")
	}
	if created.CreateTask.Task.Status != "open" || created.CreateTask.Task.DueOn != "2026-08-07" {
		t.Errorf("task = %+v, want open on 2026-08-07", created.CreateTask.Task)
	}
	wantPayload := map[string]any{
		"id":       created.CreateTask.Task.ID,
		"title":    "Call Maria",
		"status":   "open",
		"due_on":   "2026-08-07",
		"priority": 2,
	}
	if h.events.countOf(event.TaskCreated) != 1 {
		t.Fatalf("task.created count = %d, want 1", h.events.countOf(event.TaskCreated))
	}
	if diff := cmp.Diff(wantPayload, h.events.payloads[0]); diff != "" {
		t.Errorf("task.created payload mismatch (-want +got):\n%s", diff)
	}
}

func TestCreateTaskOriginRules(t *testing.T) {
	t.Parallel()

	h := newMutationHarness(t)
	originEventID := uuid.Must(uuid.NewV7())

	sessionCaller, err := h.client.RawPost(fmt.Sprintf(`mutation { createTask(input: {
		title: "Automated", dueOn: "2026-08-07", originEventId: %q
	}) { task { id } replay } }`, originEventID))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, sessionCaller.Errors); got != "VALIDATION" {
		t.Errorf("session origin code = %q, want VALIDATION", got)
	}

	tokenClient := h.originClient(t, "n8n production")
	var first createdTask
	tokenClient.MustPost(fmt.Sprintf(`mutation { createTask(input: {
		title: "Automated", dueOn: "2026-08-07", originEventId: %q
	}) { task { id } replay } }`, originEventID), &first)
	if first.CreateTask.Replay {
		t.Error("first origin create replay = true, want false")
	}

	var replayed createdTask
	tokenClient.MustPost(fmt.Sprintf(`mutation { createTask(input: {
		title: "Automated", dueOn: "2026-08-07", originEventId: %q
	}) { task { id } replay } }`, originEventID), &replayed)

	if !replayed.CreateTask.Replay {
		t.Error("replayed create replay = false, want true")
	}
	if replayed.CreateTask.Task.ID != first.CreateTask.Task.ID {
		t.Errorf("replayed id = %s, want the first task %s", replayed.CreateTask.Task.ID, first.CreateTask.Task.ID)
	}
	if got := h.events.countOf(event.TaskCreated); got != 1 {
		t.Errorf("task.created count = %d, want 1, a replay never republishes", got)
	}
}

func TestUpdateTaskLifecycle(t *testing.T) {
	t.Parallel()

	h := newMutationHarness(t)
	var created createdTask
	h.client.MustPost(`mutation { createTask(input: {
		title: "Call Maria", dueOn: "2026-08-07"
	}) { task { id } replay } }`, &created)
	taskID := created.CreateTask.Task.ID

	var done struct {
		UpdateTask struct {
			Status string `json:"status"`
		} `json:"updateTask"`
	}
	h.client.MustPost(fmt.Sprintf(
		`mutation { updateTask(id: %q, input: {status: "done"}) { status } }`, taskID,
	), &done)

	if done.UpdateTask.Status != "done" {
		t.Errorf("status = %q, want done", done.UpdateTask.Status)
	}
	if got := h.events.countOf(event.TaskCompleted); got != 1 {
		t.Fatalf("task.completed count = %d, want 1", got)
	}

	h.client.MustPost(fmt.Sprintf(
		`mutation { updateTask(id: %q, input: {title: "Call Maria Perez"}) { status } }`, taskID,
	), &done)
	if got := h.events.countOf(event.TaskCompleted); got != 1 {
		t.Errorf("task.completed count after a title edit = %d, want still 1", got)
	}

	badStatus, err := h.client.RawPost(fmt.Sprintf(
		`mutation { updateTask(id: %q, input: {status: "paused"}) { status } }`, taskID,
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, badStatus.Errors); got != "VALIDATION" {
		t.Errorf("bad status code = %q, want VALIDATION", got)
	}

	missing, err := h.client.RawPost(fmt.Sprintf(
		`mutation { updateTask(id: %q, input: {status: "done"}) { status } }`, uuid.Must(uuid.NewV7()),
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, missing.Errors); got != "NOT_FOUND" {
		t.Errorf("missing task code = %q, want NOT_FOUND", got)
	}
}

func TestWebhookLifecycle(t *testing.T) {
	t.Parallel()

	h := newMutationHarness(t)

	var created struct {
		CreateWebhook struct {
			Webhook struct {
				ID     string   `json:"id"`
				URL    string   `json:"url"`
				Events []string `json:"events"`
			} `json:"webhook"`
			Secret string `json:"secret"`
		} `json:"createWebhook"`
	}
	h.client.MustPost(`mutation { createWebhook(
		url: "https://example.com/hook", events: ["task.created"]
	) { webhook { id url events } secret } }`, &created)

	if created.CreateWebhook.Secret == "" {
		t.Error("secret is empty, want the one time signing secret")
	}
	if created.CreateWebhook.Webhook.URL != "https://example.com/hook" {
		t.Errorf("url = %q, want the subscribed endpoint", created.CreateWebhook.Webhook.URL)
	}

	var listed struct {
		Webhooks []struct {
			ID string `json:"id"`
		} `json:"webhooks"`
	}
	h.client.MustPost(`{ webhooks { id } }`, &listed)
	if len(listed.Webhooks) != 1 || listed.Webhooks[0].ID != created.CreateWebhook.Webhook.ID {
		t.Errorf("webhooks = %+v, want the one subscription", listed.Webhooks)
	}

	otherUserClient := newGraphClient(t, h.resolver, uuid.Must(uuid.NewV7()))
	var othersListing struct {
		Webhooks []struct {
			ID string `json:"id"`
		} `json:"webhooks"`
	}
	otherUserClient.MustPost(`{ webhooks { id } }`, &othersListing)
	if len(othersListing.Webhooks) != 0 {
		t.Errorf("another user's webhooks = %+v, want none", othersListing.Webhooks)
	}

	var deleted struct {
		DeleteWebhook bool `json:"deleteWebhook"`
	}
	h.client.MustPost(fmt.Sprintf(
		`mutation { deleteWebhook(id: %q) }`, created.CreateWebhook.Webhook.ID,
	), &deleted)
	if !deleted.DeleteWebhook {
		t.Error("deleteWebhook = false, want true")
	}

	badURL, err := h.client.RawPost(
		`mutation { createWebhook(url: "ftp://example.com", events: ["task.created"]) { secret } }`,
	)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, badURL.Errors); got != "VALIDATION" {
		t.Errorf("bad url code = %q, want VALIDATION", got)
	}

	unknownEvent, err := h.client.RawPost(
		`mutation { createWebhook(url: "https://example.com/hook", events: ["task.exploded"]) { secret } }`,
	)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, unknownEvent.Errors); got != "VALIDATION" {
		t.Errorf("unknown event code = %q, want VALIDATION", got)
	}
}

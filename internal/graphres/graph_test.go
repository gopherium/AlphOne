// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/cursor"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/plugins/fields"
	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// composedRoot composes the core resolver with graph plugins whose lazy
// pools never connect, mirroring the binary's graph root assembly.
func composedRoot(t *testing.T, resolver *graphres.Resolver) graph.ResolverRoot {
	t.Helper()
	root, err := graphroot.FromPlugins(resolver, lazyGraphPlugins(t))
	if err != nil {
		t.Fatalf("graphroot.FromPlugins() error = %v, want nil", err)
	}
	return root
}

// lazyGraphPlugins returns one instance of every graph plugin over an unreachable database.
func lazyGraphPlugins(t *testing.T) []sdk.Plugin {
	t.Helper()
	deps := sdk.Deps{DatabaseURL: "postgres://graph:graph@localhost:1/graph"}
	whatsappPlugin, err := whatsapp.Register(deps)
	if err != nil {
		t.Fatalf("whatsapp.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = whatsappPlugin.Stop(context.Background()) })
	importerPlugin, err := importer.Register(deps)
	if err != nil {
		t.Fatalf("importer.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = importerPlugin.Stop(context.Background()) })
	fieldsPlugin, err := fields.Register(deps)
	if err != nil {
		t.Fatalf("fields.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = fieldsPlugin.Stop(context.Background()) })
	return []sdk.Plugin{whatsappPlugin, importerPlugin, fieldsPlugin}
}

// newTestPool returns a pgxpool over a fresh migrated test database.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.CoreMigrator())
	pool, err := pgxpool.New(t.Context(), cfg.URL())
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newGraphHandler returns the gated gqlgen server over the composed resolver root.
func newGraphHandler(t *testing.T, resolver *graphres.Resolver) *handler.Server {
	t.Helper()
	srv := handler.New(graphres.ExecutableSchema(composedRoot(t, resolver)))
	srv.AddTransport(transport.POST{})
	srv.Use(extension.FixedComplexityLimit(graphres.ComplexityLimit))
	srv.AroundOperations(graphres.AnonymousGate)
	srv.SetErrorPresenter(graphres.PresentError)
	return srv
}

// newDecoratedGraphClient returns a test client whose request context passes through decorate.
func newDecoratedGraphClient(
	t *testing.T, resolver *graphres.Resolver, decorate func(context.Context) context.Context,
) *gqlclient.Client {
	t.Helper()
	srv := newGraphHandler(t, resolver)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := sdk.WithRequestScope(decorate(r.Context()), sdk.NewRequestScope())
		srv.ServeHTTP(w, r.WithContext(ctx))
	})
	return gqlclient.New(wrapped)
}

// authkitIdentity stamps ctx with the acting identity.
func authkitIdentity(ctx context.Context, id uuid.UUID) context.Context {
	return authkit.WithIdentity(ctx, authkit.Identity{ID: id})
}

// newGraphClient returns a gqlgen test client over the resolver, acting as assignee.
func newGraphClient(t *testing.T, resolver *graphres.Resolver, assignee uuid.UUID) *gqlclient.Client {
	t.Helper()
	return newDecoratedGraphClient(t, resolver, func(ctx context.Context) context.Context {
		return authkitIdentity(ctx, assignee)
	})
}

// newDBResolver returns a resolver over real postgres stores plus the stores.
func newDBResolver(t *testing.T) (*graphres.Resolver, *postgres.ContactStore, *postgres.TaskStore) {
	t.Helper()
	pool := newTestPool(t)
	contacts := postgres.NewContactStore(pool)
	tasks := postgres.NewTaskStore(pool)
	resolver := &graphres.Resolver{Version: "9.9.9", Contacts: contacts, Tasks: tasks}
	return resolver, contacts, tasks
}

// mustSeedContact stores a named contact and returns it.
func mustSeedContact(t *testing.T, store *postgres.ContactStore, name string) contact.Contact {
	t.Helper()
	seeded, err := contact.New(name)
	if err != nil {
		t.Fatalf("contact.New(%q) error = %v, want nil", name, err)
	}
	if err := store.Create(t.Context(), seeded); err != nil {
		t.Fatalf("Create(%q) error = %v, want nil", name, err)
	}
	return seeded
}

// mustSeedTask stores a task and returns it.
func mustSeedTask(t *testing.T, store *postgres.TaskStore, input task.Input) task.Task {
	t.Helper()
	created, err := task.New(input)
	if err != nil {
		t.Fatalf("task.New() error = %v, want nil", err)
	}
	stored, _, err := store.Create(t.Context(), created)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	return stored
}

// firstErrorCode extracts the first error extension code of a raw response.
func firstErrorCode(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var parsed []struct {
		Extensions struct {
			Code string `json:"code"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed) == 0 {
		t.Fatalf("no errors in response: %s (%v)", raw, err)
	}
	return parsed[0].Extensions.Code
}

type connectionResult struct {
	Edges []struct {
		Node struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Title string `json:"title"`
			DueOn string `json:"dueOn"`
		} `json:"node"`
		Cursor string `json:"cursor"`
	} `json:"edges"`
	PageInfo struct {
		HasNextPage     bool    `json:"hasNextPage"`
		HasPreviousPage bool    `json:"hasPreviousPage"`
		StartCursor     *string `json:"startCursor"`
		EndCursor       *string `json:"endCursor"`
	} `json:"pageInfo"`
}

func TestContactsConnectionPagesWithTheEncodedCursor(t *testing.T) {
	t.Parallel()

	resolver, contacts, _ := newDBResolver(t)
	ada := mustSeedContact(t, contacts, "Ada Lovelace")
	john := mustSeedContact(t, contacts, "John Doe")
	maria := mustSeedContact(t, contacts, "Maria Perez")
	_ = ada
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var page struct {
		Contacts connectionResult `json:"contacts"`
	}
	client.MustPost(`{ contacts(first: 2) {
		edges { node { id name } cursor }
		pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
	} }`, &page)

	if len(page.Contacts.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(page.Contacts.Edges))
	}
	if got := page.Contacts.Edges[0].Node.Name; got != "Ada Lovelace" {
		t.Errorf("first name = %q, want Ada Lovelace", got)
	}
	if !page.Contacts.PageInfo.HasNextPage {
		t.Error("hasNextPage = false, want true")
	}
	if page.Contacts.PageInfo.HasPreviousPage {
		t.Error("hasPreviousPage = true, want the constant false")
	}
	wantEnd := cursor.EncodeContact(john)
	if page.Contacts.PageInfo.EndCursor == nil || *page.Contacts.PageInfo.EndCursor != wantEnd {
		t.Errorf("endCursor = %v, want the encoded cursor %q", page.Contacts.PageInfo.EndCursor, wantEnd)
	}

	var rest struct {
		Contacts connectionResult `json:"contacts"`
	}
	client.MustPost(fmt.Sprintf(`{ contacts(first: 2, after: %q) {
		edges { node { id name } cursor }
		pageInfo { hasNextPage endCursor }
	} }`, wantEnd), &rest)

	if len(rest.Contacts.Edges) != 1 || rest.Contacts.Edges[0].Node.ID != maria.ID.String() {
		t.Fatalf("second page = %+v, want only Maria Perez", rest.Contacts.Edges)
	}
	if rest.Contacts.PageInfo.HasNextPage {
		t.Error("second page hasNextPage = true, want false")
	}
}

func TestContactDetailResolvesTheForceResolverFields(t *testing.T) {
	t.Parallel()

	resolver, contacts, tasks := newDBResolver(t)
	maria := mustSeedContact(t, contacts, "Maria Perez")
	identity, err := contact.NewIdentity(maria.ID, "phone", "184467235", "Maria")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}
	if err := contacts.AddIdentity(t.Context(), identity); err != nil {
		t.Fatalf("AddIdentity() error = %v, want nil", err)
	}
	assignee := uuid.Must(uuid.NewV7())
	mustSeedTask(t, tasks, task.Input{
		Title: "Call Maria", DueOn: time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC),
		AssigneeID: assignee, ContactID: maria.ID,
	})
	stored, err := contacts.Get(t.Context(), maria.ID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	client := newGraphClient(t, resolver, assignee)

	var detail struct {
		Contact struct {
			Name       string `json:"name"`
			CreatedAt  string `json:"createdAt"`
			Identities []struct {
				Channel    string `json:"channel"`
				Identifier string `json:"identifier"`
			} `json:"identities"`
			Tasks connectionResult `json:"tasks"`
		} `json:"contact"`
	}
	client.MustPost(fmt.Sprintf(`{ contact(id: %q) {
		name createdAt
		identities { channel identifier }
		tasks(first: 10) { edges { node { title } } pageInfo { hasNextPage } }
	} }`, maria.ID), &detail)

	if detail.Contact.Name != "Maria Perez" {
		t.Errorf("name = %q, want Maria Perez", detail.Contact.Name)
	}
	if want := stored.CreatedAt.UTC().Format(time.RFC3339Nano); detail.Contact.CreatedAt != want {
		t.Errorf("createdAt = %q, want %q", detail.Contact.CreatedAt, want)
	}
	if len(detail.Contact.Identities) != 1 || detail.Contact.Identities[0].Identifier != "184467235" {
		t.Errorf("identities = %+v, want the phone identity", detail.Contact.Identities)
	}
	if len(detail.Contact.Tasks.Edges) != 1 || detail.Contact.Tasks.Edges[0].Node.Title != "Call Maria" {
		t.Errorf("tasks = %+v, want Call Maria", detail.Contact.Tasks.Edges)
	}
}

func TestTasksConnectionFiltersAndResolvesContacts(t *testing.T) {
	t.Parallel()

	resolver, contacts, tasks := newDBResolver(t)
	maria := mustSeedContact(t, contacts, "Maria Perez")
	assignee := uuid.Must(uuid.NewV7())
	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	for i := range 3 {
		mustSeedTask(t, tasks, task.Input{
			Title: fmt.Sprintf("Task %d", i), DueOn: day, AssigneeID: assignee, ContactID: maria.ID,
		})
	}
	client := newGraphClient(t, resolver, assignee)

	var page struct {
		Tasks struct {
			Edges []struct {
				Node struct {
					Title   string `json:"title"`
					DueOn   string `json:"dueOn"`
					Contact *struct {
						Name string `json:"name"`
					} `json:"contact"`
				} `json:"node"`
			} `json:"edges"`
			PageInfo struct {
				HasNextPage bool    `json:"hasNextPage"`
				EndCursor   *string `json:"endCursor"`
			} `json:"pageInfo"`
		} `json:"tasks"`
	}
	client.MustPost(`{ tasks(date: "2026-08-06", first: 2) {
		edges { node { title dueOn contact { name } } }
		pageInfo { hasNextPage endCursor }
	} }`, &page)

	if len(page.Tasks.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(page.Tasks.Edges))
	}
	if !page.Tasks.PageInfo.HasNextPage {
		t.Error("hasNextPage = false, want true")
	}
	first := page.Tasks.Edges[0].Node
	if first.DueOn != "2026-08-06" {
		t.Errorf("dueOn = %q, want the Date scalar 2026-08-06", first.DueOn)
	}
	if first.Contact == nil || first.Contact.Name != "Maria Perez" {
		t.Errorf("contact = %+v, want Maria Perez", first.Contact)
	}
}

func TestTasksScopesByAssigneeExceptWhenFilteredByContact(t *testing.T) {
	t.Parallel()

	resolver, contacts, tasks := newDBResolver(t)
	maria := mustSeedContact(t, contacts, "Maria Perez")
	owner := uuid.Must(uuid.NewV7())
	caller := uuid.Must(uuid.NewV7())
	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	mustSeedTask(t, tasks, task.Input{
		Title: "Call Maria Perez about the renewal", DueOn: day, AssigneeID: owner, ContactID: maria.ID,
	})
	client := newGraphClient(t, resolver, caller)

	var listed struct {
		Tasks struct {
			Edges []struct {
				Node struct {
					Title string `json:"title"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"tasks"`
	}
	client.MustPost(`{ tasks(dueBefore: "2026-08-07", status: "all", first: 10) {
		edges { node { title } }
	} }`, &listed)
	if len(listed.Tasks.Edges) != 0 {
		t.Errorf("dueBefore listed %+v, want another assignee's task withheld", listed.Tasks.Edges)
	}

	client.MustPost(fmt.Sprintf(`{ tasks(contactId: %q, status: "all", first: 10) {
		edges { node { title } }
	} }`, maria.ID), &listed)

	if len(listed.Tasks.Edges) != 1 {
		t.Fatalf("contactId listed %d tasks, want the contact's work whoever holds it", len(listed.Tasks.Edges))
	}
	if listed.Tasks.Edges[0].Node.Title != "Call Maria Perez about the renewal" {
		t.Errorf("task = %q, want the other assignee's task", listed.Tasks.Edges[0].Node.Title)
	}
}

func TestTasksRejectsFilterViolations(t *testing.T) {
	t.Parallel()

	resolver, _, _ := newDBResolver(t)
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	noFilter, err := client.RawPost(`{ tasks { edges { node { id } } pageInfo { hasNextPage } } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, noFilter.Errors); got != "VALIDATION" {
		t.Errorf("no filter code = %q, want VALIDATION", got)
	}

	twoFilters, err := client.RawPost(fmt.Sprintf(
		`{ tasks(date: "2026-08-06", contactId: %q) { pageInfo { hasNextPage } } }`, uuid.Must(uuid.NewV7()),
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, twoFilters.Errors); got != "VALIDATION" {
		t.Errorf("two filters code = %q, want VALIDATION", got)
	}
}

func TestContactQueryErrorPaths(t *testing.T) {
	t.Parallel()

	resolver, _, _ := newDBResolver(t)
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	missing, err := client.RawPost(fmt.Sprintf(`{ contact(id: %q) { name } }`, uuid.Must(uuid.NewV7())))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, missing.Errors); got != "NOT_FOUND" {
		t.Errorf("missing contact code = %q, want NOT_FOUND", got)
	}

	badCursor, err := client.RawPost(`{ contacts(after: "%%%") { pageInfo { hasNextPage } } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, badCursor.Errors); got != "VALIDATION" {
		t.Errorf("malformed cursor code = %q, want VALIDATION", got)
	}
}

// stubContactStore counts batch loads over a fixed contact set.
type stubContactStore struct {
	contacts       map[uuid.UUID]contact.Contact
	batches        [][]uuid.UUID
	listErr        error
	listByIDsErr   error
	identitiesErr  error
	addIdentityErr error
}

func (s *stubContactStore) Get(_ context.Context, id uuid.UUID) (contact.Contact, error) {
	c, ok := s.contacts[id]
	if !ok {
		return contact.Contact{}, contact.ErrNotFound
	}
	return c, nil
}

func (s *stubContactStore) ListContacts(
	_ context.Context, _, _, _ string, _ uuid.UUID, _ int,
) ([]contact.Contact, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return nil, nil
}

func (s *stubContactStore) ListContactIdentities(_ context.Context, _ uuid.UUID) ([]contact.Identity, error) {
	if s.identitiesErr != nil {
		return nil, s.identitiesErr
	}
	return nil, nil
}

func (s *stubContactStore) ListByIDs(_ context.Context, ids []uuid.UUID) ([]contact.Contact, error) {
	if s.listByIDsErr != nil {
		return nil, s.listByIDsErr
	}
	s.batches = append(s.batches, ids)
	var found []contact.Contact
	for _, id := range ids {
		if c, ok := s.contacts[id]; ok {
			found = append(found, c)
		}
	}
	return found, nil
}

func (s *stubContactStore) Create(_ context.Context, _ contact.Contact) error {
	return nil
}

func (s *stubContactStore) CreateContactWithIdentities(
	_ context.Context, _ contact.Contact, _ []contact.Identity,
) error {
	return nil
}

func (s *stubContactStore) RenameContact(_ context.Context, _ uuid.UUID, _ string) (contact.Contact, error) {
	return contact.Contact{}, contact.ErrNotFound
}

func (s *stubContactStore) AddIdentity(_ context.Context, _ contact.Identity) error {
	return s.addIdentityErr
}

func (s *stubContactStore) DeleteIdentity(_ context.Context, _, _ uuid.UUID) error {
	return nil
}

// stubTaskStore serves a fixed task list for every day listing.
type stubTaskStore struct {
	tasks             []task.Task
	createErr         error
	updateErr         error
	listForContactErr error
}

func (s *stubTaskStore) Get(_ context.Context, id uuid.UUID) (task.Task, error) {
	for _, stored := range s.tasks {
		if stored.ID == id {
			return stored, nil
		}
	}
	return task.Task{}, task.ErrNotFound
}

func (s *stubTaskStore) ListForDay(
	_ context.Context, _ uuid.UUID, _ time.Time, _ string, _ task.Page,
) ([]task.Task, error) {
	return s.tasks, nil
}

func (s *stubTaskStore) ListDueBefore(
	_ context.Context, _ uuid.UUID, _ time.Time, _ string, _ task.Page,
) ([]task.Task, error) {
	return nil, nil
}

func (s *stubTaskStore) ListForContact(
	_ context.Context, _ uuid.UUID, _ string, _ task.Page,
) ([]task.Task, error) {
	if s.listForContactErr != nil {
		return nil, s.listForContactErr
	}
	return nil, nil
}

func (s *stubTaskStore) Create(_ context.Context, t task.Task) (task.Task, bool, error) {
	if s.createErr != nil {
		return task.Task{}, false, s.createErr
	}
	return t, true, nil
}

func (s *stubTaskStore) Update(_ context.Context, t task.Task) (task.Task, error) {
	if s.updateErr != nil {
		return task.Task{}, s.updateErr
	}
	return t, nil
}

func TestTaskContactsAreLoadedInOneBatch(t *testing.T) {
	t.Parallel()

	maria := contact.Contact{ID: uuid.Must(uuid.NewV7()), Name: "Maria Perez", CreatedAt: time.Now()}
	john := contact.Contact{ID: uuid.Must(uuid.NewV7()), Name: "John Doe", CreatedAt: time.Now()}
	contactStub := &stubContactStore{contacts: map[uuid.UUID]contact.Contact{maria.ID: maria, john.ID: john}}
	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	taskStub := &stubTaskStore{tasks: []task.Task{
		{ID: uuid.Must(uuid.NewV7()), Title: "One", DueOn: day, ContactID: maria.ID},
		{ID: uuid.Must(uuid.NewV7()), Title: "Two", DueOn: day, ContactID: john.ID},
		{ID: uuid.Must(uuid.NewV7()), Title: "Three", DueOn: day, ContactID: maria.ID},
	}}
	resolver := &graphres.Resolver{
		Version: "9.9.9", Contacts: contactStub, Tasks: taskStub, BatchWait: 100 * time.Millisecond,
	}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var page struct {
		Tasks struct {
			Edges []struct {
				Node struct {
					Contact *struct {
						Name string `json:"name"`
					} `json:"contact"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"tasks"`
	}
	client.MustPost(`{ tasks(date: "2026-08-06") { edges { node { contact { name } } } } }`, &page)

	if len(contactStub.batches) != 1 {
		t.Fatalf("batch loads = %d, want 1", len(contactStub.batches))
	}
	if got := len(contactStub.batches[0]); got != 2 {
		t.Errorf("batched ids = %d, want the 2 distinct contacts", got)
	}
	if len(page.Tasks.Edges) != 3 || page.Tasks.Edges[0].Node.Contact.Name != "Maria Perez" {
		t.Errorf("edges = %+v, want 3 tasks with resolved contacts", page.Tasks.Edges)
	}
}

func TestStoreFailuresAreMaskedAsInternal(t *testing.T) {
	t.Parallel()

	contactStub := &stubContactStore{listErr: fmt.Errorf("pgx: connection refused")}
	resolver := &graphres.Resolver{Version: "9.9.9", Contacts: contactStub, Tasks: &stubTaskStore{}}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	response, err := client.RawPost(`{ contacts { pageInfo { hasNextPage } } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", got)
	}
	if strings.Contains(string(response.Errors), "pgx") {
		t.Errorf("errors leak the store failure: %s", response.Errors)
	}
}

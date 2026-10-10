// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
)

// storedInOrder stores one contact per name, each created a minute after the one before.
func storedInOrder(t *testing.T, store *postgres.ContactStore, names ...string) {
	t.Helper()
	start := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	for i, name := range names {
		held := contact.Contact{
			ID: uuid.Must(uuid.NewV7()), Name: name, CreatedAt: start.Add(time.Duration(i) * time.Minute),
		}
		if err := store.Create(t.Context(), held); err != nil {
			t.Fatalf("storing %q: %v", name, err)
		}
	}
}

// pagedNames returns the names of one contact page, failing the test on any error.
func pagedNames(t *testing.T, store *postgres.ContactStore, filter contact.Filter, page contact.Page) []string {
	t.Helper()
	listed, err := store.PageContacts(t.Context(), filter, page)
	if err != nil {
		t.Fatalf("PageContacts(%+v, %+v) error = %v, want nil", filter, page, err)
	}
	names := make([]string, 0, len(listed))
	for _, held := range listed {
		names = append(names, held.Name)
	}
	return names
}

func TestPageContactsWalksTheDirectoryByOffsetInNameOrder(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	storedInOrder(t, store, "Maria Perez", "Ada Lovelace", "Grace Hopper")

	walks := map[string]struct {
		offset int
		want   []string
	}{
		"the first page":      {offset: 0, want: []string{"Ada Lovelace", "Grace Hopper"}},
		"the rest":            {offset: 2, want: []string{"Maria Perez"}},
		"a page past the end": {offset: 10, want: []string{}},
	}
	for name, walk := range walks {
		got := pagedNames(t, store, contact.Filter{}, contact.Page{Limit: 2, Offset: walk.offset})
		if !slices.Equal(got, walk.want) {
			t.Errorf("%s = %v, want %v", name, got, walk.want)
		}
	}
}

func TestPageContactsSortsByNameOrCreationEitherWay(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	storedInOrder(t, store, "Maria Perez", "Ada Lovelace", "Grace Hopper")

	sorts := map[string]struct {
		page contact.Page
		want []string
	}{
		"name descending": {contact.Page{Descending: true}, []string{"Maria Perez", "Grace Hopper", "Ada Lovelace"}},
		"oldest first":    {contact.Page{ByCreated: true}, []string{"Maria Perez", "Ada Lovelace", "Grace Hopper"}},
		"newest first": {
			contact.Page{ByCreated: true, Descending: true}, []string{"Grace Hopper", "Ada Lovelace", "Maria Perez"},
		},
	}
	for name, sort := range sorts {
		sort.page.Limit = 10
		if got := pagedNames(t, store, contact.Filter{}, sort.page); !slices.Equal(got, sort.want) {
			t.Errorf("%s = %v, want %v", name, got, sort.want)
		}
	}
}

func TestPageContactsBreaksATieByID(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	start := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	var ids []uuid.UUID
	for range 3 {
		held := contact.Contact{ID: uuid.Must(uuid.NewV7()), Name: "Maria Perez", CreatedAt: start}
		if err := store.Create(t.Context(), held); err != nil {
			t.Fatalf("storing a namesake: %v", err)
		}
		ids = append(ids, held.ID)
	}

	reversed := slices.Clone(ids)
	slices.Reverse(reversed)
	orders := []struct {
		page contact.Page
		want []uuid.UUID
	}{
		{contact.Page{Limit: 10}, ids},
		{contact.Page{ByCreated: true, Limit: 10}, ids},
		{contact.Page{Descending: true, Limit: 10}, reversed},
		{contact.Page{ByCreated: true, Descending: true, Limit: 10}, reversed},
	}
	for _, order := range orders {
		listed, err := store.PageContacts(t.Context(), contact.Filter{}, order.page)
		if err != nil {
			t.Fatalf("PageContacts() error = %v, want nil", err)
		}
		got := make([]uuid.UUID, 0, len(listed))
		for _, held := range listed {
			got = append(got, held.ID)
		}
		if !slices.Equal(got, order.want) {
			t.Errorf("namesakes under %+v = %v, want %v", order.page, got, order.want)
		}
	}
}

func TestContactPagesAndTheirCountAgreeOnTheFilter(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	ctx := t.Context()
	maria := mustContact(t, "Maria Perez")
	if err := store.CreateContactWithIdentity(ctx, maria, mustIdentity(t, maria.ID, "whatsapp", "184467235")); err != nil {
		t.Fatalf("seeding Maria Perez: %v", err)
	}
	ada := mustContact(t, "Ada")
	adaIdentity, err := contact.NewIdentity(ada.ID, "email", "ada@example.com", "Ada Lovelace")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}
	if err := store.CreateContactWithIdentity(ctx, ada, adaIdentity); err != nil {
		t.Fatalf("seeding Ada: %v", err)
	}
	if err := store.Create(ctx, mustContact(t, "Bruno")); err != nil {
		t.Fatalf("seeding Bruno: %v", err)
	}

	filters := map[string]struct {
		filter contact.Filter
		want   []string
	}{
		"everyone":               {contact.Filter{}, []string{"Ada", "Bruno", "Maria Perez"}},
		"a name":                 {contact.Filter{Query: "mar"}, []string{"Maria Perez"}},
		"an identifier's digits": {contact.Filter{Query: "184 467", Digits: "184467"}, []string{"Maria Perez"}},
		"a display name":         {contact.Filter{Query: "lovelace"}, []string{"Ada"}},
		"a channel":              {contact.Filter{Channels: []string{"whatsapp"}}, []string{"Maria Perez"}},
		"two channels":           {contact.Filter{Channels: []string{"whatsapp", "email"}}, []string{"Ada", "Maria Perez"}},
		"a search off the channel": {
			contact.Filter{Query: "ada", Channels: []string{"whatsapp"}}, []string{},
		},
	}
	for name, tt := range filters {
		if got := pagedNames(t, store, tt.filter, contact.Page{Limit: 10}); !slices.Equal(got, tt.want) {
			t.Errorf("%s pages %v, want %v", name, got, tt.want)
		}
		total, err := store.CountContacts(ctx, tt.filter)
		if err != nil {
			t.Fatalf("%s CountContacts() error = %v, want nil", name, err)
		}
		if total != len(tt.want) {
			t.Errorf("%s counts %d, want %d", name, total, len(tt.want))
		}
	}
}

func TestContactSearchesTakePatternCharactersLiterally(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	ctx := t.Context()
	storedInOrder(t, store, "Promo 50% off", "Promo 500 off", "team_lead", "team lead", `back\slash`, "backslash")
	displayNames := map[string]string{"percent display name": "Sale 30% now", "digit display name": "Sale 300 now"}
	for name, displayName := range displayNames {
		held := mustContact(t, name)
		identity, err := contact.NewIdentity(held.ID, "email", strings.Fields(name)[0]+"@example.com", displayName)
		if err != nil {
			t.Fatalf("NewIdentity() error = %v, want nil", err)
		}
		if err := store.CreateContactWithIdentity(ctx, held, identity); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}

	searches := map[string]struct {
		query string
		want  []string
	}{
		"a percent sign":                   {"50%", []string{"Promo 50% off"}},
		"an underscore":                    {"m_l", []string{"team_lead"}},
		"a backslash":                      {`k\s`, []string{`back\slash`}},
		"a percent sign in a display name": {"30%", []string{"percent display name"}},
	}
	for name, tt := range searches {
		filter := contact.Filter{Query: tt.query}
		if got := pagedNames(t, store, filter, contact.Page{Limit: 10}); !slices.Equal(got, tt.want) {
			t.Errorf("%s pages %v, want %v", name, got, tt.want)
		}
		total, err := store.CountContacts(ctx, filter)
		if err != nil {
			t.Fatalf("%s CountContacts() error = %v, want nil", name, err)
		}
		if total != len(tt.want) {
			t.Errorf("%s counts %d, want %d", name, total, len(tt.want))
		}
		listed, err := store.ListContacts(ctx, tt.query, "", "", uuid.Nil, 10)
		if err != nil {
			t.Fatalf("%s ListContacts() error = %v, want nil", name, err)
		}
		names := make([]string, 0, len(listed))
		for _, held := range listed {
			names = append(names, held.Name)
		}
		if !slices.Equal(names, tt.want) {
			t.Errorf("%s lists %v, want %v", name, names, tt.want)
		}
	}
}

func TestAContactPageStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	acme := seededTenant(t, pool)
	storedContact(t, store, standingIn(t, acme), "Maria Perez")
	storedContact(t, store, t.Context(), "Ada Lovelace")

	listed, err := store.PageContacts(standingIn(t, acme), contact.Filter{}, contact.Page{Limit: 10})
	if err != nil {
		t.Fatalf("PageContacts() error = %v, want nil", err)
	}
	total, err := store.CountContacts(standingIn(t, acme), contact.Filter{})
	if err != nil {
		t.Fatalf("CountContacts() error = %v, want nil", err)
	}

	if len(listed) != 1 || listed[0].Name != "Maria Perez" || total != 1 {
		t.Errorf("page = %+v counting %d, want only the tenant's own contact", listed, total)
	}
}

func TestListContactIdentitiesAnswersEveryRequestedContact(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	ctx := t.Context()
	maria := mustContact(t, "Maria Perez")
	if err := store.CreateContactWithIdentities(ctx, maria, []contact.Identity{
		mustIdentity(t, maria.ID, "whatsapp", "184467235"),
		mustIdentity(t, maria.ID, "email", "maria@example.com"),
	}); err != nil {
		t.Fatalf("seeding Maria Perez: %v", err)
	}
	ada := mustContact(t, "Ada Lovelace")
	if err := store.CreateContactWithIdentity(ctx, ada, mustIdentity(t, ada.ID, "email", "ada@example.com")); err != nil {
		t.Fatalf("seeding Ada Lovelace: %v", err)
	}
	unrequested := mustContact(t, "Grace Hopper")
	unrequestedIdentity := mustIdentity(t, unrequested.ID, "email", "grace@example.com")
	if err := store.CreateContactWithIdentity(ctx, unrequested, unrequestedIdentity); err != nil {
		t.Fatalf("seeding Grace Hopper: %v", err)
	}

	identities, err := store.ListContactIdentities(ctx, []uuid.UUID{maria.ID, ada.ID})
	if err != nil {
		t.Fatalf("ListContactIdentities() error = %v, want nil", err)
	}

	byContact := map[uuid.UUID][]string{}
	for _, held := range identities {
		byContact[held.ContactID] = append(byContact[held.ContactID], held.Identifier)
	}
	if got := byContact[maria.ID]; !slices.Equal(got, []string{"maria@example.com", "184467235"}) {
		t.Errorf("Maria Perez identities = %v, want both in channel order", got)
	}
	if got := byContact[ada.ID]; !slices.Equal(got, []string{"ada@example.com"}) {
		t.Errorf("Ada Lovelace identities = %v, want her one address", got)
	}
	if len(byContact) != 2 {
		t.Errorf("identities answered for %d contacts, want only the 2 requested", len(byContact))
	}
}

// contactFilterOf returns the WHERE clause of one named query up to its ORDER BY.
func contactFilterOf(t *testing.T, query string) string {
	t.Helper()
	_, filter, found := strings.Cut(query, "WHERE")
	if !found {
		t.Fatalf("query holds no WHERE clause: %s", query)
	}
	filter, _, _ = strings.Cut(filter, "ORDER BY")
	return strings.TrimSuffix(strings.TrimSpace(filter), ";")
}

func TestTheContactPageAndItsCountShareOneFilter(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatalf("reading queries.sql: %v", err)
	}
	queries := namedQueries(string(source))

	page, count := contactFilterOf(t, queries["PageContacts"]), contactFilterOf(t, queries["CountContacts"])

	if page != count {
		t.Errorf("the page filters by\n%s\nand the count by\n%s\nwant one filter", page, count)
	}
}

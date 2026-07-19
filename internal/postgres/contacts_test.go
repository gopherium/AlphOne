// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/testdb"
)

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

func TestContactStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := mustContact(t, "María Pérez")

	if err := store.Create(t.Context(), maria); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	got, err := store.Get(t.Context(), maria.ID)

	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if diff := cmp.Diff(maria, got, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("Get() mismatch (-want +got):\n%s", diff)
	}
}

func TestContactStoreGetMissingContact(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))

	_, err := store.Get(t.Context(), uuid.Must(uuid.NewV7()))

	if !errors.Is(err, contact.ErrNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, contact.ErrNotFound)
	}
}

func TestContactStoreReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	maria := mustContact(t, "María Pérez")
	pool.Close()

	if err := store.Create(t.Context(), maria); err == nil {
		t.Error("Create() on closed pool error = nil, want error")
	}
	if _, err := store.Get(t.Context(), maria.ID); err == nil || errors.Is(err, contact.ErrNotFound) {
		t.Errorf("Get() on closed pool error = %v, want a non-ErrNotFound error", err)
	}
	if _, err := store.ListContacts(t.Context(), "", "", "", uuid.Nil, 10); err == nil {
		t.Error("ListContacts() on closed pool error = nil, want error")
	}
}

func TestListContactsWalksPagesInDictionaryOrder(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	for _, name := range []string{"maría", "Ana", "zoe", "Ángel", "Ana", "Bruno"} {
		if err := store.Create(t.Context(), mustContact(t, name)); err != nil {
			t.Fatalf("creating %q: %v", name, err)
		}
	}

	var collected []contact.Contact
	afterName, afterID := "", uuid.Nil
	for {
		page, err := store.ListContacts(t.Context(), "", "", afterName, afterID, 2)
		if err != nil {
			t.Fatalf("ListContacts() error = %v, want nil", err)
		}
		collected = append(collected, page...)
		if len(page) < 2 {
			break
		}
		last := page[len(page)-1]
		afterName, afterID = last.Name, last.ID
	}

	wantOrder := []string{"Ana", "Ana", "Ángel", "Bruno", "maría", "zoe"}
	if got, want := len(collected), len(wantOrder); got != want {
		t.Fatalf("collected %d contacts over the walk, want %d", got, want)
	}
	for i, want := range wantOrder {
		if collected[i].Name != want {
			t.Errorf("collected[%d].Name = %q, want %q", i, collected[i].Name, want)
		}
	}
	if collected[0].ID.String() > collected[1].ID.String() {
		t.Error("duplicate names not tie-broken by id")
	}
	seen := make(map[uuid.UUID]bool, len(collected))
	for _, c := range collected {
		if seen[c.ID] {
			t.Errorf("contact %s returned twice across pages", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestListContactsSearches(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	ctx := t.Context()
	maria := mustContact(t, "María Pérez")
	if err := store.CreateContactWithIdentity(ctx, maria, mustIdentity(t, maria.ID, "whatsapp", "184467235")); err != nil {
		t.Fatalf("seeding María: %v", err)
	}
	angel := mustContact(t, "Ángel")
	if err := store.Create(ctx, angel); err != nil {
		t.Fatalf("seeding Ángel: %v", err)
	}
	bruno := mustContact(t, "Bruno")
	brunoIdentity := mustIdentity(t, bruno.ID, "whatsapp", "15551999887@lid")
	if err := store.CreateContactWithIdentity(ctx, bruno, brunoIdentity); err != nil {
		t.Fatalf("seeding Bruno: %v", err)
	}
	ada := mustContact(t, "Ada")
	adaIdentity, err := contact.NewIdentity(ada.ID, "whatsapp", "142000333", "Ada Lovelace")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}
	if err := store.CreateContactWithIdentity(ctx, ada, adaIdentity); err != nil {
		t.Fatalf("seeding Ada: %v", err)
	}

	tests := map[string]struct {
		query  string
		digits string
		want   []string
	}{
		"name substring":              {query: "mar", want: []string{"María Pérez"}},
		"accented name":               {query: "Áng", want: []string{"Ángel"}},
		"uppercase name":              {query: "BRU", want: []string{"Bruno"}},
		"identifier by digits":        {query: "+1 844 672", digits: "1844672", want: []string{"María Pérez"}},
		"lid identifier by digits":    {query: "1555 1999", digits: "15551999", want: []string{"Bruno"}},
		"display name":                {query: "lovelace", want: []string{"Ada"}},
		"digitless skips identifiers": {query: "@lid", want: nil},
		"no match":                    {query: "zzz", want: nil},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			page, err := store.ListContacts(t.Context(), tc.query, tc.digits, "", uuid.Nil, 50)
			if err != nil {
				t.Fatalf("ListContacts(%q) error = %v, want nil", tc.query, err)
			}

			if len(page) != len(tc.want) {
				t.Fatalf("ListContacts(%q) returned %d contacts, want %d", tc.query, len(page), len(tc.want))
			}
			for i, want := range tc.want {
				if page[i].Name != want {
					t.Errorf("page[%d].Name = %q, want %q", i, page[i].Name, want)
				}
			}
		})
	}
}

func TestListContactsSearchComposesWithPagination(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	for _, name := range []string{"Ana García", "Ana López", "Bruno"} {
		if err := store.Create(t.Context(), mustContact(t, name)); err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
	}

	first, err := store.ListContacts(t.Context(), "ana", "", "", uuid.Nil, 1)
	if err != nil {
		t.Fatalf("first page error = %v, want nil", err)
	}
	if len(first) != 1 || first[0].Name != "Ana García" {
		t.Fatalf("first page = %+v, want Ana García", first)
	}

	second, err := store.ListContacts(t.Context(), "ana", "", first[0].Name, first[0].ID, 1)
	if err != nil {
		t.Fatalf("second page error = %v, want nil", err)
	}
	if len(second) != 1 || second[0].Name != "Ana López" {
		t.Fatalf("second page = %+v, want Ana López", second)
	}

	third, err := store.ListContacts(t.Context(), "ana", "", second[0].Name, second[0].ID, 1)
	if err != nil {
		t.Fatalf("third page error = %v, want nil", err)
	}
	if len(third) != 0 {
		t.Fatalf("third page = %+v, want empty", third)
	}
}

func TestListContactIdentitiesOrdersByChannelAndIdentifier(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	ada := mustContact(t, "Ada")
	if err := store.CreateContactWithIdentity(t.Context(), ada, mustIdentity(t, ada.ID, "whatsapp", "200111222")); err != nil {
		t.Fatalf("seeding Ada: %v", err)
	}
	extra := []contact.Identity{
		mustIdentity(t, ada.ID, "email", "ada@example.com"),
		mustIdentity(t, ada.ID, "whatsapp", "100333444"),
	}
	for _, identity := range extra {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			identity.ID, identity.ContactID, string(identity.Channel), identity.Identifier,
			identity.DisplayName, identity.CreatedAt,
		); err != nil {
			t.Fatalf("inserting identity %q: %v", identity.Identifier, err)
		}
	}

	identities, err := store.ListContactIdentities(t.Context(), ada.ID)

	if err != nil {
		t.Fatalf("ListContactIdentities() error = %v, want nil", err)
	}
	want := []string{"ada@example.com", "100333444", "200111222"}
	if len(identities) != len(want) {
		t.Fatalf("len(identities) = %d, want %d", len(identities), len(want))
	}
	for i, identifier := range want {
		if identities[i].Identifier != identifier {
			t.Errorf("identities[%d].Identifier = %q, want %q", i, identities[i].Identifier, identifier)
		}
	}
}

func TestListContactIdentitiesReturnsEmptyForBareContacts(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	bruno := mustContact(t, "Bruno")
	if err := store.Create(t.Context(), bruno); err != nil {
		t.Fatalf("seeding Bruno: %v", err)
	}

	identities, err := store.ListContactIdentities(t.Context(), bruno.ID)

	if err != nil {
		t.Fatalf("ListContactIdentities() error = %v, want nil", err)
	}
	if len(identities) != 0 {
		t.Fatalf("len(identities) = %d, want 0", len(identities))
	}
}

func TestListContactsReturnsAnEmptyPageOnAnEmptyTable(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))

	page, err := store.ListContacts(t.Context(), "", "", "", uuid.Nil, 50)

	if err != nil {
		t.Fatalf("ListContacts() error = %v, want nil", err)
	}
	if len(page) != 0 {
		t.Fatalf("len(page) = %d, want 0", len(page))
	}
}

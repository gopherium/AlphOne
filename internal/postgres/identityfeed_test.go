// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/testdb"
)

const (
	feedSettleDeadline = 30 * time.Second
	feedPause          = 20 * time.Millisecond
	feedPageLimit      = 50
	restoredLead       = 1_000_000_000
)

// feedReader reads one identity feed in the tenant its context serves, failing the test on any error.
type feedReader struct {
	t    *testing.T
	ctx  context.Context
	feed *contact.Feed
}

// page returns one page of the email feed after the cursor.
func (r feedReader) page(after contact.Cursor, limit int) ([]contact.AddedIdentity, contact.Cursor) {
	r.t.Helper()
	answered, next, err := r.feed.IdentitiesAddedSince(r.ctx, "email", after, limit)
	if err != nil {
		r.t.Fatalf("IdentitiesAddedSince() error = %v, want nil", err)
	}
	return answered, next
}

// settled reads the email feed after the cursor until it answers at least want identities or the deadline passes.
func (r feedReader) settled(after contact.Cursor, want int) ([]contact.AddedIdentity, contact.Cursor) {
	r.t.Helper()
	deadline := time.Now().Add(feedSettleDeadline)
	for {
		answered, next := r.page(after, feedPageLimit)
		if len(answered) >= want || time.Now().After(deadline) {
			return answered, next
		}
		time.Sleep(feedPause)
	}
}

// identifiersOf returns the identifier of every answered identity, in order.
func identifiersOf(answered []contact.AddedIdentity) []string {
	identifiers := make([]string, len(answered))
	for i, added := range answered {
		identifiers[i] = added.Identifier
	}
	return identifiers
}

// addedEmail attaches a new email identity to the contact, failing the test on any error.
func addedEmail(t *testing.T, store *postgres.ContactStore, owner uuid.UUID, address string) {
	t.Helper()
	if err := store.AddIdentity(t.Context(), mustIdentity(t, owner, "email", address)); err != nil {
		t.Fatalf("AddIdentity(%s) error = %v, want nil", address, err)
	}
}

func TestTheFeedAnswersIdentitiesFromEveryWritePathInTheOrderTheyWereAdded(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := mustContact(t, "Maria Perez")
	created := mustIdentity(t, maria.ID, "email", "maria@example.com")
	err := store.CreateContactWithIdentities(t.Context(), maria, []contact.Identity{
		created, mustIdentity(t, maria.ID, "phone", "+184467235"),
	})
	if err != nil {
		t.Fatalf("CreateContactWithIdentities() error = %v, want nil", err)
	}
	attached := mustIdentity(t, maria.ID, "email", "maria.perez@example.com")
	if err := store.AddIdentity(t.Context(), attached); err != nil {
		t.Fatalf("AddIdentity() error = %v, want nil", err)
	}
	if _, _, err := contact.NewResolver(store).CreateWithIdentities(t.Context(), "Ada Lovelace",
		[]contact.Address{{Channel: "email", Identifier: "Ada@Example.com"}}); err != nil {
		t.Fatalf("CreateWithIdentities() error = %v, want nil", err)
	}
	imported, err := store.LookupIdentity(t.Context(), "email", "ada@example.com")
	if err != nil {
		t.Fatalf("LookupIdentity() error = %v, want the imported identity", err)
	}

	answered, _ := feedReader{t, t.Context(), contact.NewFeed(store, feedPageLimit)}.settled(contact.Cursor{}, 3)

	want := []contact.AddedIdentity{
		{ContactID: maria.ID, Identifier: "maria@example.com", AddedAt: created.CreatedAt},
		{ContactID: maria.ID, Identifier: "maria.perez@example.com", AddedAt: attached.CreatedAt},
		{ContactID: imported.ContactID, Identifier: "ada@example.com", AddedAt: imported.CreatedAt},
	}
	if diff := cmp.Diff(want, answered, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("IdentitiesAddedSince() mismatch (-want +got):\n%s", diff)
	}
}

// heldIdentity begins a transaction on its own connection that stores an email identity and stays open.
func heldIdentity(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID, address string) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("beginning the held transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	held := mustIdentity(t, owner, "email", address)
	if _, err := tx.Exec(t.Context(),
		"INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)"+
			" VALUES ($1, $2, $3, $4, '', $5)",
		held.ID, held.ContactID, string(held.Channel), held.Identifier, held.CreatedAt); err != nil {
		t.Fatalf("storing the held identity: %v", err)
	}
	return tx
}

func TestTheFeedAnswersALateCommitBeforeTheLaterIdentityItWaitedBehind(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	reader := feedReader{t, t.Context(), contact.NewFeed(store, feedPageLimit)}
	maria := mustContact(t, "Maria Perez")
	if err := store.Create(t.Context(), maria); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	held := heldIdentity(t, pool, maria.ID, "maria@example.com")
	addedEmail(t, store, maria.ID, "maria.perez@example.com")

	early, cursor := reader.page(contact.Cursor{}, feedPageLimit)
	if err := held.Commit(t.Context()); err != nil {
		t.Fatalf("committing the held transaction: %v", err)
	}
	late, _ := reader.settled(cursor, 2)

	if len(early) != 0 {
		t.Errorf("the feed answered %v while an older transaction was open, want it to wait", identifiersOf(early))
	}
	want := []string{"maria@example.com", "maria.perez@example.com"}
	if diff := cmp.Diff(want, identifiersOf(late)); diff != "" {
		t.Errorf("the feed after the commit mismatch (-want +got):\n%s", diff)
	}
}

func TestTheFeedResumesFromItsCursorWithoutGapsOrRepeats(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	reader := feedReader{t, t.Context(), contact.NewFeed(store, feedPageLimit)}
	maria := mustContact(t, "Maria Perez")
	if err := store.Create(t.Context(), maria); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	want := []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com", "e@example.com"}
	for _, address := range want[:3] {
		addedEmail(t, store, maria.ID, address)
	}
	reader.settled(contact.Cursor{}, 3)

	first, cursor := reader.page(contact.Cursor{}, 2)
	second, cursor := reader.page(cursor, 2)
	for _, address := range want[3:] {
		addedEmail(t, store, maria.ID, address)
	}
	rest, cursor := reader.settled(cursor, 2)
	after, unchanged := reader.page(cursor, 2)

	var got []string
	for _, page := range [][]contact.AddedIdentity{first, second, rest} {
		got = append(got, identifiersOf(page)...)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the pages read in turn mismatch (-want +got):\n%s", diff)
	}
	if len(after) != 0 || unchanged != cursor {
		t.Errorf("a read past the end answered %v and moved the cursor to %+v, want nothing at %+v",
			identifiersOf(after), unchanged, cursor)
	}
}

func TestAReaderStartingAtTheHorizonNeverSeesAnOlderIdentity(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	feed := contact.NewFeed(store, feedPageLimit)
	reader := feedReader{t, t.Context(), feed}
	maria := mustContact(t, "Maria Perez")
	if err := store.Create(t.Context(), maria); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	addedEmail(t, store, maria.ID, "maria@example.com")
	reader.settled(contact.Cursor{}, 1)

	horizon, err := feed.Horizon(t.Context())
	if err != nil {
		t.Fatalf("Horizon() error = %v, want nil", err)
	}
	addedEmail(t, store, maria.ID, "maria.perez@example.com")
	answered, _ := reader.settled(horizon, 1)

	if diff := cmp.Diff([]string{"maria.perez@example.com"}, identifiersOf(answered)); diff != "" {
		t.Errorf("the feed from the horizon mismatch (-want +got):\n%s", diff)
	}
}

// serverTransactionEnd returns the transaction number the database server behind pool hands out next.
func serverTransactionEnd(t *testing.T, pool *pgxpool.Pool) uint64 {
	t.Helper()
	var end uint64
	if err := pool.QueryRow(t.Context(), "SELECT pg_snapshot_xmax(pg_current_snapshot())").Scan(&end); err != nil {
		t.Fatalf("reading the next transaction number: %v", err)
	}
	return end
}

func TestTheFeedRefusesACursorPastEveryTransactionTheServerBegan(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	cursor := contact.Cursor{Transaction: serverTransactionEnd(t, pool) + restoredLead, Identity: uuid.Must(uuid.NewV7())}

	answered, next, err := contact.NewFeed(postgres.NewContactStore(pool), feedPageLimit).
		IdentitiesAddedSince(t.Context(), "email", cursor, feedPageLimit)

	if !errors.Is(err, contact.ErrCursorAhead) {
		t.Errorf("IdentitiesAddedSince() error = %v, want %v", err, contact.ErrCursorAhead)
	}
	if answered != nil || next != cursor {
		t.Errorf("IdentitiesAddedSince() = %v at %+v, want nothing at %+v", identifiersOf(answered), next, cursor)
	}
}

func TestTheFeedTakesACursorAtTheNextTransactionTheServerHandsOut(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	cursor := contact.Cursor{Transaction: serverTransactionEnd(t, pool)}

	_, next, err := contact.NewFeed(postgres.NewContactStore(pool), feedPageLimit).
		IdentitiesAddedSince(t.Context(), "email", cursor, feedPageLimit)

	if err != nil || next != cursor {
		t.Errorf("IdentitiesAddedSince() = %+v, %v, want %+v and nil, the cursor Horizon answers on an idle server",
			next, err, cursor)
	}
}

// stampedPastTheServer stamps every identity of the contact with a transaction the server behind pool has not reached.
func stampedPastTheServer(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"UPDATE core.contact_identities SET added_xid = $1 WHERE contact_id = $2",
		serverTransactionEnd(t, pool)+restoredLead, owner); err != nil {
		t.Fatalf("stamping the identities past the server: %v", err)
	}
}

func TestMigrateBringsAnIdentityRestoredFromAServerFurtherAlongBackIntoTheFeed(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	databaseURL := testdb.New(t, testdb.Migrator())
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	store := postgres.NewContactStore(pool)
	feed := contact.NewFeed(store, feedPageLimit)
	reader := feedReader{t, t.Context(), feed}
	maria := storedOwnerOf(t, store, ownedAddress{"Maria Perez", "email", "maria@example.com"})
	stampedPastTheServer(t, pool, maria.ID)
	following, err := feed.Horizon(t.Context())
	if err != nil {
		t.Fatalf("Horizon() error = %v, want nil", err)
	}

	if err := postgres.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	fromStart, _ := reader.settled(contact.Cursor{}, 1)
	fromHorizon, _ := reader.settled(following, 1)

	want := []string{"maria@example.com"}
	if diff := cmp.Diff(want, identifiersOf(fromStart)); diff != "" {
		t.Errorf("the feed from the start after Migrate() mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(want, identifiersOf(fromHorizon)); diff != "" {
		t.Errorf("the feed of a reader following before Migrate() mismatch (-want +got):\n%s", diff)
	}
}

func TestTheFeedReportsAConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	pool.Close()

	if _, _, err := store.IdentitiesAddedSince(t.Context(), "email", contact.Cursor{}, 1); err == nil {
		t.Error("IdentitiesAddedSince() on a closed pool error = nil, want the failure")
	}
	if _, err := store.IdentityHorizon(t.Context()); err == nil {
		t.Error("IdentityHorizon() on a closed pool error = nil, want the failure")
	}
	if _, err := store.IdentityCursorAhead(t.Context(), contact.Cursor{}); err == nil {
		t.Error("IdentityCursorAhead() on a closed pool error = nil, want the failure")
	}
}

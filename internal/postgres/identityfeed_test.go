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

// serverIdentity returns the system identifier of the database server behind pool.
func serverIdentity(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var server int64
	if err := pool.QueryRow(t.Context(), "SELECT system_identifier FROM pg_control_system()").Scan(&server); err != nil {
		t.Fatalf("reading the server identity: %v", err)
	}
	return server
}

// serversOf returns the server that numbered each identity of the contact, keyed by identifier.
func serversOf(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID) map[string]int64 {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		"SELECT identifier, added_server FROM core.contact_identities WHERE contact_id = $1", owner)
	if err != nil {
		t.Fatalf("reading the servers that numbered the identities: %v", err)
	}
	servers := map[string]int64{}
	var identifier string
	var server int64
	if _, err := pgx.ForEachRow(rows, []any{&identifier, &server}, func() error {
		servers[identifier] = server
		return nil
	}); err != nil {
		t.Fatalf("reading the servers that numbered the identities: %v", err)
	}
	return servers
}

func TestEveryWritePathStampsAnAddressWithTheServerThatNumberedIt(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	maria := storedOwnerOf(t, store, ownedAddress{"Maria Perez", "email", "maria@example.com"})
	addedEmail(t, store, maria.ID, "maria.perez@example.com")

	server := serverIdentity(t, pool)
	want := map[string]int64{"maria@example.com": server, "maria.perez@example.com": server}
	if diff := cmp.Diff(want, serversOf(t, pool, maria.ID)); diff != "" {
		t.Errorf("the server of each stored address mismatch (-want +got):\n%s", diff)
	}
}

func TestTheFeedRefusesACursorPastEveryTransactionTheServerBegan(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	cursor := contact.Cursor{
		Server:      serverIdentity(t, pool),
		Transaction: serverTransactionEnd(t, pool) + restoredLead,
		Identity:    uuid.Must(uuid.NewV7()),
	}

	answered, next, err := contact.NewFeed(postgres.NewContactStore(pool), feedPageLimit).
		IdentitiesAddedSince(t.Context(), "email", cursor, feedPageLimit)

	if !errors.Is(err, contact.ErrCursorStale) {
		t.Errorf("IdentitiesAddedSince() error = %v, want %v", err, contact.ErrCursorStale)
	}
	if answered != nil || next != cursor {
		t.Errorf("IdentitiesAddedSince() = %v at %+v, want nothing at %+v", identifiersOf(answered), next, cursor)
	}
}

func TestTheFeedTakesACursorAtTheNextTransactionTheServerHandsOut(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	cursor := contact.Cursor{Server: serverIdentity(t, pool), Transaction: serverTransactionEnd(t, pool)}

	_, next, err := contact.NewFeed(postgres.NewContactStore(pool), feedPageLimit).
		IdentitiesAddedSince(t.Context(), "email", cursor, feedPageLimit)

	if err != nil || next != cursor {
		t.Errorf("IdentitiesAddedSince() = %+v, %v, want %+v and nil, the cursor Horizon answers on an idle server",
			next, err, cursor)
	}
}

func TestTheFeedRefusesACursorAnotherServerNumbered(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"when no address waits past it":      false,
		"when a newer address waits past it": true,
	}

	for name, newer := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := newTestPool(t)
			store := postgres.NewContactStore(pool)
			feed := contact.NewFeed(store, feedPageLimit)
			here := contact.Cursor{
				Server:      serverIdentity(t, pool),
				Transaction: serverTransactionEnd(t, pool) - 1,
				Identity:    uuid.Must(uuid.NewV7()),
			}
			elsewhere := here
			elsewhere.Server++
			if newer {
				storedOwnerOf(t, store, ownedAddress{"Maria Perez", "email", "maria@example.com"})
				if waiting, _ := (feedReader{t, t.Context(), feed}).settled(here, 1); len(waiting) != 1 {
					t.Fatalf("the feed past this server's cursor answered %v, want maria@example.com", identifiersOf(waiting))
				}
			}

			answered, next, err := feed.IdentitiesAddedSince(t.Context(), "email", elsewhere, feedPageLimit)

			if !errors.Is(err, contact.ErrCursorStale) {
				t.Errorf("IdentitiesAddedSince() error = %v, want %v", err, contact.ErrCursorStale)
			}
			if answered != nil || next != elsewhere {
				t.Errorf("IdentitiesAddedSince() = %v at %+v, want nothing at %+v", identifiersOf(answered), next, elsewhere)
			}
		})
	}
}

func TestTheFeedNamesItsServerInTheCursorsItAnswers(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	feed := contact.NewFeed(store, feedPageLimit)
	storedOwnerOf(t, store, ownedAddress{"Maria Perez", "email", "maria@example.com"})

	answered, next := feedReader{t, t.Context(), feed}.settled(contact.Cursor{}, 1)
	horizon, err := feed.Horizon(t.Context())

	if err != nil {
		t.Fatalf("Horizon() error = %v, want nil", err)
	}
	server := serverIdentity(t, pool)
	if len(answered) != 1 || next.Server != server {
		t.Errorf("a page of %v answered a cursor of the server %d, want the server %d",
			identifiersOf(answered), next.Server, server)
	}
	if horizon.Server != server {
		t.Errorf("Horizon() answered a cursor of the server %d, want the server %d", horizon.Server, server)
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

// migratedPool returns the address of a fresh migrated test database and a pool over it, skipping in short mode.
func migratedPool(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	databaseURL := testdb.New(t, testdb.Migrator())
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return databaseURL, pool
}

func TestMigrateBringsAnIdentityRestoredFromAServerFurtherAlongBackIntoTheFeed(t *testing.T) {
	t.Parallel()

	databaseURL, pool := migratedPool(t)
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

// numberedElsewhere stamps the identity with the transaction as another server numbered it.
func numberedElsewhere(t *testing.T, pool *pgxpool.Pool, identifier string, transaction uint64) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"UPDATE core.contact_identities SET added_server = $1, added_xid = $2 WHERE identifier = $3",
		serverIdentity(t, pool)+1, transaction, identifier); err != nil {
		t.Fatalf("stamping the identity as another server numbered it: %v", err)
	}
}

func TestMigrateBringsAnAddressAnotherServerSavedBackIntoTheFeedOfAReaderThisServerAnswered(t *testing.T) {
	t.Parallel()

	databaseURL, pool := migratedPool(t)
	store := postgres.NewContactStore(pool)
	reader := feedReader{t, t.Context(), contact.NewFeed(store, feedPageLimit)}
	maria := storedOwnerOf(t, store, ownedAddress{"Maria Perez", "email", "maria@example.com"})
	_, following := reader.settled(contact.Cursor{}, 1)
	addedEmail(t, store, maria.ID, "maria.perez@example.com")
	numberedElsewhere(t, pool, "maria.perez@example.com", following.Transaction-1)
	before, _ := reader.page(following, feedPageLimit)

	if err := postgres.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	after, _ := reader.settled(following, 1)

	if len(before) != 0 {
		t.Errorf("the feed from the cursor before Migrate() answered %v, want nothing", identifiersOf(before))
	}
	if diff := cmp.Diff([]string{"maria.perez@example.com"}, identifiersOf(after)); diff != "" {
		t.Errorf("the feed from the cursor after Migrate() mismatch (-want +got):\n%s", diff)
	}
	server := serverIdentity(t, pool)
	want := map[string]int64{"maria@example.com": server, "maria.perez@example.com": server}
	if diff := cmp.Diff(want, serversOf(t, pool, maria.ID)); diff != "" {
		t.Errorf("the server of each address after Migrate() mismatch (-want +got):\n%s", diff)
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
	if _, err := store.IdentityCursorCheck(t.Context(), contact.Cursor{}); err == nil {
		t.Error("IdentityCursorCheck() on a closed pool error = nil, want the failure")
	}
}

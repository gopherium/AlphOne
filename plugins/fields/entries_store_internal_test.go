// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// roomy is a limit no store test reaches unless it means to.
const roomy = 10

// noted builds the cells of one entry holding only a comment.
func noted(comments ...string) []map[string]any {
	entries := make([]map[string]any, len(comments))
	for at, comment := range comments {
		entries[at] = map[string]any{"comment": comment}
	}
	return entries
}

// added stores entries on a contact's history in the tenant the context serves, returning them with their ids.
func added(t *testing.T, p *Plugin, ctx context.Context, contactID uuid.UUID, comments ...string) []map[string]any {
	t.Helper()
	stored, err := p.store.addEntry(ctx, contactID, "history", noted(comments...), roomy)
	if err != nil {
		t.Fatalf("addEntry() error = %v, want nil", err)
	}
	return stored
}

// idOf returns the id one stored entry carries.
func idOf(t *testing.T, entry map[string]any) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(entry[entryIDKey].(string))
	if err != nil {
		t.Fatalf("entry id %v: %v", entry[entryIDKey], err)
	}
	return id
}

// historyHeld reads the comments and ids a contact's history holds, in stored order.
func historyHeld(t *testing.T, p *Plugin, ctx context.Context, contactID uuid.UUID) ([]string, []string) {
	t.Helper()
	listed, _ := storedValues(t, p, ctx, contactID)["history"].([]any)
	comments := make([]string, 0, len(listed))
	ids := make([]string, 0, len(listed))
	for _, entry := range listed {
		cells := entry.(map[string]any)
		comment, _ := cells["comment"].(string)
		comments = append(comments, comment)
		id, _ := cells[entryIDKey].(string)
		ids = append(ids, id)
	}
	return comments, ids
}

// wantHistory fails unless a contact's history holds exactly the given comments in order.
func wantHistory(t *testing.T, p *Plugin, ctx context.Context, contactID uuid.UUID, want ...string) {
	t.Helper()
	if held, _ := historyHeld(t, p, ctx, contactID); !slices.Equal(held, want) {
		t.Errorf("history = %q, want %q", held, want)
	}
}

// foreignContact stores a contact in the default tenant and returns a context serving another tenant.
func foreignContact(t *testing.T, p *Plugin) (context.Context, uuid.UUID) {
	t.Helper()
	return inTenant(t, p), seedContact(t, p, "Maria Perez")
}

func TestAddEntryStoresTheFirstEntryOnAContactWithNoRow(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")

	stored, err := p.store.addEntry(t.Context(), contactID, "history",
		[]map[string]any{{"date": "2026-09-01", "comment": "First call."}}, roomy)

	if err != nil {
		t.Fatalf("addEntry() error = %v, want nil", err)
	}
	if len(stored) != 1 || idOf(t, stored[0]).Version() != 7 || stored[0]["date"] != "2026-09-01" {
		t.Fatalf("addEntry() = %v, want the entry answered under a UUIDv7 id", stored)
	}
	comments, ids := historyHeld(t, p, t.Context(), contactID)
	if !slices.Equal(comments, []string{"First call."}) || ids[0] != stored[0][entryIDKey] {
		t.Errorf("history = %q %q, want the answered entry stored", comments, ids)
	}
}

func TestAddEntryPutsTheLastGivenOnTop(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")

	stored := added(t, p, t.Context(), contactID, "one", "two", "three")

	wantHistory(t, p, t.Context(), contactID, "three", "two", "one")
	if stored[0]["comment"] != "three" {
		t.Errorf("addEntry() = %v, want the answer in stored order", stored)
	}
	_, ids := historyHeld(t, p, t.Context(), contactID)
	if len(slices.Compact(slices.Sorted(slices.Values(ids)))) != 3 {
		t.Errorf("ids = %q, want each entry under its own id", ids)
	}
}

func TestAddEntryPrependsToAHeldList(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	added(t, p, t.Context(), contactID, "first")

	added(t, p, t.Context(), contactID, "second")

	wantHistory(t, p, t.Context(), contactID, "second", "first")
}

func TestAddEntryLeavesOtherKeysAlone(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	visits := []any{map[string]any{"id": "kept", "place": "Office"}}
	if err := p.store.writeValues(t.Context(), contactID,
		map[string]any{"birthDate": "1990-04-17", "visits": visits}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}

	added(t, p, t.Context(), contactID, "call")

	held := storedValues(t, p, t.Context(), contactID)
	if held["birthDate"] != "1990-04-17" || len(held["visits"].([]any)) != 1 {
		t.Errorf("values = %v, want the other keys untouched", held)
	}
}

func TestEntriesKeepEachRepeaterApart(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	added(t, p, t.Context(), contactID, "call")
	visits, err := p.store.addEntry(t.Context(), contactID, "visits", []map[string]any{{"place": "Office"}}, roomy)
	if err != nil {
		t.Fatalf("addEntry() to visits error = %v, want nil", err)
	}

	if err := p.store.deleteEntry(t.Context(), contactID, "visits", idOf(t, visits[0])); err != nil {
		t.Fatalf("deleteEntry() from visits error = %v, want nil", err)
	}

	held := storedValues(t, p, t.Context(), contactID)
	if _, kept := held["visits"]; kept {
		t.Errorf("values = %v, want the emptied visits dropped", held)
	}
	wantHistory(t, p, t.Context(), contactID, "call")
}

func TestAddEntryRefusesAContactNoTenantHolds(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	unknown := uuid.Must(uuid.NewV7())

	_, err := p.store.addEntry(t.Context(), unknown, "history", noted("call"), roomy)

	if !errors.Is(err, errNoContact) {
		t.Errorf("addEntry() error = %v, want errNoContact", err)
	}
}

func TestAddEntryRefusesAnotherTenantsContact(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme, contactID := foreignContact(t, p)

	_, err := p.store.addEntry(acme, contactID, "history", noted("call"), roomy)

	if !errors.Is(err, errNoContact) {
		t.Fatalf("addEntry() error = %v, want errNoContact", err)
	}
	if held := storedValues(t, p, acme, contactID); held != nil {
		t.Errorf("Acme's values = %v, want no row written", held)
	}
	if held := storedValues(t, p, t.Context(), contactID); held != nil {
		t.Errorf("the owner's values = %v, want no row written", held)
	}
}

func TestAddEntryRefusesAForeignContactUnderAStrayRow(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme, contactID := foreignContact(t, p)
	stray := []any{map[string]any{"id": "stray", "comment": "stray"}}
	if err := p.store.writeValues(acme, contactID, map[string]any{"history": stray}); err != nil {
		t.Fatalf("writeValues() in Acme error = %v, want nil", err)
	}

	_, err := p.store.addEntry(acme, contactID, "history", noted("call"), roomy)

	if !errors.Is(err, errNoContact) {
		t.Fatalf("addEntry() error = %v, want errNoContact", err)
	}
	wantHistory(t, p, acme, contactID, "stray")
}

func TestAddEntryRefusesAListAtTheLimit(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	if _, err := p.store.addEntry(t.Context(), contactID, "history", noted("one", "two"), 2); err != nil {
		t.Fatalf("addEntry() up to the limit error = %v, want nil", err)
	}

	_, err := p.store.addEntry(t.Context(), contactID, "history", noted("three"), 2)

	if !errors.Is(err, errEntriesFull) {
		t.Fatalf("addEntry() error = %v, want errEntriesFull", err)
	}
	wantHistory(t, p, t.Context(), contactID, "two", "one")
}

func TestAddEntryRefusesEntriesThatWouldPassTheLimit(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	added(t, p, t.Context(), contactID, "one")

	_, err := p.store.addEntry(t.Context(), contactID, "history", noted("two", "three"), 2)

	if !errors.Is(err, errEntriesFull) {
		t.Fatalf("addEntry() error = %v, want errEntriesFull", err)
	}
	wantHistory(t, p, t.Context(), contactID, "one")
}

func TestAddEntryRefusesAFirstBatchOverTheLimit(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")

	_, err := p.store.addEntry(t.Context(), contactID, "history", noted("one", "two"), 1)

	if !errors.Is(err, errEntriesFull) {
		t.Fatalf("addEntry() error = %v, want errEntriesFull", err)
	}
	if held := storedValues(t, p, t.Context(), contactID); held != nil {
		t.Errorf("values = %v, want no row written", held)
	}
}

func TestAddEntryTakesTheLastPlaceUnderTheLimit(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	added(t, p, t.Context(), contactID, "one")

	if _, err := p.store.addEntry(t.Context(), contactID, "history", noted("two"), 2); err != nil {
		t.Fatalf("addEntry() error = %v, want the last place taken", err)
	}

	wantHistory(t, p, t.Context(), contactID, "two", "one")
}

func TestUpdateEntryReplacesOnlyItsCells(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	stored, err := p.store.addEntry(t.Context(), contactID, "history", []map[string]any{
		{"comment": "one"}, {"date": "2026-09-01", "comment": "two"}, {"comment": "three"},
	}, roomy)
	if err != nil {
		t.Fatalf("addEntry() error = %v, want nil", err)
	}
	_, before := historyHeld(t, p, t.Context(), contactID)

	err = p.store.updateEntry(t.Context(), contactID, "history",
		idOf(t, stored[1]), map[string]any{"comment": "two edited"})

	if err != nil {
		t.Fatalf("updateEntry() error = %v, want nil", err)
	}
	wantHistory(t, p, t.Context(), contactID, "three", "two edited", "one")
	if _, after := historyHeld(t, p, t.Context(), contactID); !slices.Equal(after, before) {
		t.Errorf("ids = %q, want %q kept", after, before)
	}
	if edited := storedValues(t, p, t.Context(), contactID)["history"].([]any)[1]; edited.(map[string]any)["date"] != nil {
		t.Errorf("edited entry = %v, want every cell replaced", edited)
	}
}

func TestUpdateEntryRefusesAnIDItCannotFind(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	stored := added(t, p, t.Context(), contactID, "one")
	if err := p.store.writeValues(t.Context(), contactID,
		map[string]any{"visits": []any{map[string]any{"id": uuid.NewString(), "place": "Office"}}}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}
	bare := seedContact(t, p, "Maria Perez")
	cases := map[string]struct {
		contactID uuid.UUID
		name      string
		id        uuid.UUID
	}{
		"an unknown id":           {contactID, "history", uuid.Must(uuid.NewV7())},
		"an id under another key": {contactID, "visits", idOf(t, stored[0])},
		"a contact with no list":  {bare, "history", idOf(t, stored[0])},
	}
	for name, held := range cases {
		t.Run(name, func(t *testing.T) {
			err := p.store.updateEntry(t.Context(), held.contactID, held.name, held.id, map[string]any{"comment": "x"})

			if !errors.Is(err, errNoEntry) {
				t.Errorf("updateEntry() error = %v, want errNoEntry", err)
			}
		})
	}
	wantHistory(t, p, t.Context(), contactID, "one")
}

func TestUpdateEntryStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme, contactID := foreignContact(t, p)
	stored := added(t, p, t.Context(), contactID, "one")

	err := p.store.updateEntry(acme, contactID, "history", idOf(t, stored[0]), map[string]any{"comment": "x"})

	if !errors.Is(err, errNoEntry) {
		t.Fatalf("updateEntry() error = %v, want errNoEntry", err)
	}
	wantHistory(t, p, t.Context(), contactID, "one")
}

func TestDeleteEntryDropsOnlyItsEntry(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	stored := added(t, p, t.Context(), contactID, "one", "two", "three")

	if err := p.store.deleteEntry(t.Context(), contactID, "history", idOf(t, stored[1])); err != nil {
		t.Fatalf("deleteEntry() error = %v, want nil", err)
	}

	wantHistory(t, p, t.Context(), contactID, "three", "one")
}

func TestDeleteEntryDropsTheKeyWithTheLastEntry(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	if err := p.store.writeValues(t.Context(), contactID, map[string]any{"birthDate": "1990-04-17"}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}
	stored := added(t, p, t.Context(), contactID, "one")

	if err := p.store.deleteEntry(t.Context(), contactID, "history", idOf(t, stored[0])); err != nil {
		t.Fatalf("deleteEntry() error = %v, want nil", err)
	}

	held := storedValues(t, p, t.Context(), contactID)
	if _, kept := held["history"]; kept || held["birthDate"] != "1990-04-17" {
		t.Errorf("values = %v, want only the emptied history dropped", held)
	}
}

func TestDeleteEntryRefusesAnUnknownID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	added(t, p, t.Context(), contactID, "one")

	err := p.store.deleteEntry(t.Context(), contactID, "history", uuid.Must(uuid.NewV7()))

	if !errors.Is(err, errNoEntry) {
		t.Fatalf("deleteEntry() error = %v, want errNoEntry", err)
	}
	wantHistory(t, p, t.Context(), contactID, "one")
}

func TestDeleteEntryStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme, contactID := foreignContact(t, p)
	stored := added(t, p, t.Context(), contactID, "one")

	err := p.store.deleteEntry(acme, contactID, "history", idOf(t, stored[0]))

	if !errors.Is(err, errNoEntry) {
		t.Fatalf("deleteEntry() error = %v, want errNoEntry", err)
	}
	wantHistory(t, p, t.Context(), contactID, "one")
}

func TestDeleteEntryKeepsEntriesWithoutAnID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	if err := p.store.writeValues(t.Context(), contactID, map[string]any{
		"history": []any{map[string]any{"comment": "old one"}, map[string]any{"comment": "old two"}},
	}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}
	stored := added(t, p, t.Context(), contactID, "new")

	if err := p.store.deleteEntry(t.Context(), contactID, "history", idOf(t, stored[0])); err != nil {
		t.Fatalf("deleteEntry() error = %v, want nil", err)
	}

	wantHistory(t, p, t.Context(), contactID, "old one", "old two")
}

func TestEntriesReportAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)
	id := uuid.Must(uuid.NewV7())

	if _, err := p.store.addEntry(t.Context(), id, "history", noted("one"), roomy); err == nil ||
		errors.Is(err, errNoContact) {
		t.Errorf("addEntry() error = %v, want the closed pool reported", err)
	}
	if err := p.store.updateEntry(t.Context(), id, "history", id, map[string]any{}); err == nil ||
		errors.Is(err, errNoEntry) {
		t.Errorf("updateEntry() error = %v, want the closed pool reported", err)
	}
	if err := p.store.deleteEntry(t.Context(), id, "history", id); err == nil || errors.Is(err, errNoEntry) {
		t.Errorf("deleteEntry() error = %v, want the closed pool reported", err)
	}
}

// awaitLocked waits until the given statement is blocked on a lock in the test's database.
func awaitLocked(t *testing.T, p *Plugin, statement string) {
	t.Helper()
	const query = `SELECT count(*) FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid()
			AND wait_event_type = 'Lock' AND starts_with(query, left($1, (
				SELECT setting::int - 1 FROM pg_settings WHERE name = 'track_activity_query_size')))`
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := p.pool.QueryRow(t.Context(), query, statement).Scan(&waiting); err != nil {
			t.Fatalf("reading the waiting statements: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no statement waited on the racing one: %.40s", statement)
}

// holding runs one statement in a transaction the test commits once another statement waits on it.
func holding(t *testing.T, p *Plugin, statement string, args ...any) pgx.Tx {
	t.Helper()
	racing, err := p.pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("beginning the racing transaction: %v", err)
	}
	t.Cleanup(func() { _ = racing.Rollback(context.Background()) })
	if _, err := racing.Exec(t.Context(), statement, args...); err != nil {
		t.Fatalf("racing statement: %v", err)
	}
	return racing
}

// holdingAnAdd runs the add statement for an entry noted racing in a transaction the test commits later.
func holdingAnAdd(t *testing.T, p *Plugin, contactID uuid.UUID) pgx.Tx {
	t.Helper()
	return holding(t, p, addEntryStatement,
		contactID, "history", newestFirst(noted("racing")), sdk.DefaultTenantID, roomy)
}

// release commits the racing transaction once the waiting statement is blocked, returning what it answered.
func release(t *testing.T, p *Plugin, racing pgx.Tx, waiting string, done <-chan error) error {
	t.Helper()
	awaitLocked(t, p, waiting)
	if err := racing.Commit(t.Context()); err != nil {
		t.Fatalf("committing the racing transaction: %v", err)
	}
	return <-done
}

// addingInBackground adds one entry off the test goroutine, answering on the returned channel.
func addingInBackground(p *Plugin, contactID uuid.UUID, comment string, limit int) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := p.store.addEntry(context.Background(), contactID, "history", noted(comment), limit)
		done <- err
	}()
	return done
}

func TestAddEntryKeepsARacingAdd(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	added(t, p, t.Context(), contactID, "first")
	racing := holdingAnAdd(t, p, contactID)

	done := addingInBackground(p, contactID, "waiting", roomy)

	if err := release(t, p, racing, addEntryStatement, done); err != nil {
		t.Fatalf("addEntry() error = %v, want nil", err)
	}
	wantHistory(t, p, t.Context(), contactID, "waiting", "racing", "first")
}

func TestAddEntryKeepsARacingFirstAdd(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	racing := holdingAnAdd(t, p, contactID)

	done := addingInBackground(p, contactID, "waiting", roomy)

	if err := release(t, p, racing, addEntryStatement, done); err != nil {
		t.Fatalf("addEntry() error = %v, want nil", err)
	}
	wantHistory(t, p, t.Context(), contactID, "waiting", "racing")
}

func TestUpdateEntryKeepsARacingAdd(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	target := added(t, p, t.Context(), contactID, "target")
	targetID := idOf(t, target[0])
	racing := holdingAnAdd(t, p, contactID)
	done := make(chan error, 1)

	go func() {
		done <- p.store.updateEntry(context.Background(), contactID, "history",
			targetID, map[string]any{"comment": "edited"})
	}()

	if err := release(t, p, racing, updateEntryStatement, done); err != nil {
		t.Fatalf("updateEntry() error = %v, want nil", err)
	}
	wantHistory(t, p, t.Context(), contactID, "racing", "edited")
	if _, ids := historyHeld(t, p, t.Context(), contactID); ids[1] != targetID.String() {
		t.Errorf("ids = %q, want the edited entry's id kept", ids)
	}
}

func TestDeleteEntryKeepsARacingAdd(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	target := added(t, p, t.Context(), contactID, "target")
	targetID := idOf(t, target[0])
	racing := holdingAnAdd(t, p, contactID)
	done := make(chan error, 1)

	go func() { done <- p.store.deleteEntry(context.Background(), contactID, "history", targetID) }()

	if err := release(t, p, racing, deleteEntryStatement, done); err != nil {
		t.Fatalf("deleteEntry() error = %v, want nil", err)
	}
	wantHistory(t, p, t.Context(), contactID, "racing")
}

func TestAddEntryTakesTheRoomARacingDeleteMade(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	stored, err := p.store.addEntry(t.Context(), contactID, "history", noted("one", "two"), 2)
	if err != nil {
		t.Fatalf("addEntry() error = %v, want nil", err)
	}
	racing := holding(t, p, deleteEntryStatement, contactID, "history", stored[0][entryIDKey], sdk.DefaultTenantID)

	done := addingInBackground(p, contactID, "three", 2)

	if err := release(t, p, racing, addEntryStatement, done); err != nil {
		t.Fatalf("addEntry() error = %v, want the room the delete made taken", err)
	}
	wantHistory(t, p, t.Context(), contactID, "three", "one")
}

func TestAddEntryRefusesAContactDeletedWhileItWaits(t *testing.T) {
	t.Parallel()

	for name, held := range map[string][]string{"with a stored list": {"first"}, "with no row yet": nil} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			contactID := seedContact(t, p, "Maria Perez")
			if held != nil {
				added(t, p, t.Context(), contactID, held...)
			}
			racing := holding(t, p, "DELETE FROM core.contacts WHERE id = $1", contactID)

			done := addingInBackground(p, contactID, "waiting", roomy)

			if err := release(t, p, racing, addEntryStatement, done); !errors.Is(err, errNoContact) {
				t.Errorf("addEntry() error = %v, want errNoContact", err)
			}
		})
	}
}

func TestUpdateEntryRefusesAnEntryARacingDeleteRemoved(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	stored := added(t, p, t.Context(), contactID, "one", "two")
	removedID := idOf(t, stored[1])
	racing := holding(t, p, deleteEntryStatement, contactID, "history", removedID.String(), sdk.DefaultTenantID)
	done := make(chan error, 1)

	go func() {
		done <- p.store.updateEntry(context.Background(), contactID, "history",
			removedID, map[string]any{"comment": "edited"})
	}()

	if err := release(t, p, racing, updateEntryStatement, done); !errors.Is(err, errNoEntry) {
		t.Fatalf("updateEntry() error = %v, want errNoEntry", err)
	}
	wantHistory(t, p, t.Context(), contactID, "two")
}

func TestWriteValuesKeepsARacingAdd(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	added(t, p, t.Context(), contactID, "first")
	racing := holdingAnAdd(t, p, contactID)
	done := make(chan error, 1)

	go func() {
		done <- p.store.writeValues(context.Background(), contactID, map[string]any{"birthDate": "1990-04-17"})
	}()

	if err := release(t, p, racing, "INSERT INTO plugin_fields.contact_values", done); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}
	wantHistory(t, p, t.Context(), contactID, "racing", "first")
}

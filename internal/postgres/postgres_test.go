// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/testdb"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
	checkViolation      = "23514"
)

// sqlStateOf returns the SQLSTATE err carries, empty for no error and for one that carries none.
func sqlStateOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// newTestDB returns a migrated throwaway database, skipping the test in short mode.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	return openDatabase(t, testdb.New(t, testdb.Migrator()))
}

// mustContact returns a new contact with the given name, failing the test on any error.
func mustContact(t *testing.T, name string) contact.Contact {
	t.Helper()
	c, err := contact.New(name)
	if err != nil {
		t.Fatalf("New(%q) error = %v, want nil", name, err)
	}
	return c
}

// mustIdentity returns a new identity for the given contact, failing the test on any error.
func mustIdentity(t *testing.T, contactID uuid.UUID, channel contact.Channel, identifier string) contact.Identity {
	t.Helper()
	identity, err := contact.NewIdentity(contactID, channel, identifier, "")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}
	return identity
}

// insertContact stores the contact row, failing the test on any error.
func insertContact(t *testing.T, db *sql.DB, c contact.Contact) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)",
		c.ID, c.Name, c.CreatedAt,
	)
	if err != nil {
		t.Fatalf("inserting contact %q: %v", c.Name, err)
	}
}

// insertIdentity stores the identity row and returns the database error.
func insertIdentity(db *sql.DB, identity contact.Identity) error {
	_, err := db.Exec(
		"INSERT INTO core.contact_identities "+
			"(id, contact_id, channel, identifier, display_name, created_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6)",
		identity.ID,
		identity.ContactID,
		string(identity.Channel),
		identity.Identifier,
		identity.DisplayName,
		identity.CreatedAt,
	)
	return err
}

func TestMigrationsCreateContactsNameIndex(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	var count int
	err := db.QueryRow(
		`SELECT count(*) FROM pg_indexes
		WHERE schemaname = 'core' AND tablename = 'contacts' AND indexname = 'contacts_name_id_idx'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying pg_indexes: %v", err)
	}

	if count != 1 {
		t.Fatalf("contacts_name_id_idx count = %d, want 1", count)
	}
}

// indexDefinition returns the definition of the named index on the core table, each run of whitespace one space.
func indexDefinition(t *testing.T, db *sql.DB, table, name string) string {
	t.Helper()
	var definition string
	err := db.QueryRow(
		`SELECT indexdef FROM pg_indexes WHERE schemaname = 'core' AND tablename = $1 AND indexname = $2`,
		table, name,
	).Scan(&definition)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return strings.Join(strings.Fields(definition), " ")
}

func TestMigrationsIndexTheContactsByTenantAndCreationTime(t *testing.T) {
	t.Parallel()

	definition := indexDefinition(t, newTestDB(t), "contacts", "contacts_tenant_created_id_idx")

	if !strings.HasSuffix(definition, "(tenant_id, created_at, id)") {
		t.Errorf("index definition = %q, want tenant, created time and id", definition)
	}
}

func TestMigrationsIndexTheWebhookSubscriptionsByTenantAndCreationTime(t *testing.T) {
	t.Parallel()

	definition := indexDefinition(t, newTestDB(t), "webhook_subscriptions", "webhook_subscriptions_tenant_created_id_idx")

	if !strings.HasSuffix(definition, "(tenant_id, created_at DESC, id DESC)") {
		t.Errorf("index definition = %q, want tenant, then newest first", definition)
	}
}

// dueTimeVersion is the migration giving tasks their time of day.
const dueTimeVersion = 23

// rankColumn is the rank key of every task listing index as the server spells it.
const rankColumn = "( CASE WHEN (status = 'open'::text) THEN ((- priority))::integer ELSE 0 END)"

// wantTaskIndexColumns fails the test unless each named index of core.tasks is a btree over its columns.
func wantTaskIndexColumns(t *testing.T, db *sql.DB, want map[string]string) {
	t.Helper()
	for name, columns := range want {
		if got := indexDefinition(t, db, "tasks", name); !strings.HasSuffix(got, "USING btree "+columns) {
			t.Errorf("%s definition = %q, want the columns %s", name, got, columns)
		}
	}
}

func TestMigrationsIndexTheTaskListingsByRankDayAndTime(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	clock := "( CASE WHEN (status = 'open'::text) THEN COALESCE(due_time, '24:00:00'::time without time zone) " +
		"ELSE '24:00:00'::time without time zone END)"

	wantTaskIndexColumns(t, db, map[string]string{
		"tasks_assignee_status_rank_time_idx":     "(assignee_id, status, " + rankColumn + ", due_on, " + clock + ", id)",
		"tasks_assignee_status_day_rank_time_idx": "(assignee_id, status, due_on, " + rankColumn + ", " + clock + ", id)",
		"tasks_contact_status_rank_time_idx":      "(contact_id, status, " + rankColumn + ", due_on, " + clock + ", id)",
	})
}

func TestRollingTheDueTimeMigrationBackDropsTheColumnAndRestoresTheIndexes(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	if _, err := coreProvider(t, db).DownTo(t.Context(), dueTimeVersion-1); err != nil {
		t.Fatalf("rolling back the due time migration: %v", err)
	}

	var columns int
	err := db.QueryRow(
		`SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'core' AND table_name = 'tasks' AND column_name = 'due_time'`,
	).Scan(&columns)
	if err != nil {
		t.Fatalf("querying information_schema.columns: %v", err)
	}
	if columns != 0 {
		t.Errorf("due_time columns after the rollback = %d, want 0", columns)
	}
	wantTaskIndexColumns(t, db, map[string]string{
		"tasks_assignee_status_rank_idx":     "(assignee_id, status, " + rankColumn + ", due_on, id)",
		"tasks_assignee_status_day_rank_idx": "(assignee_id, status, due_on, " + rankColumn + ", id)",
		"tasks_contact_status_rank_idx":      "(contact_id, status, " + rankColumn + ", due_on, id)",
	})
}

func TestMigrationsDropTheTaskListingIndexesWithoutATime(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	var count int
	err := db.QueryRow(
		`SELECT count(*) FROM pg_indexes WHERE schemaname = 'core' AND tablename = 'tasks'
		AND indexname IN ('tasks_assignee_status_rank_idx', 'tasks_assignee_status_day_rank_idx',
			'tasks_contact_status_rank_idx')`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying pg_indexes: %v", err)
	}

	if count != 0 {
		t.Errorf("task listing indexes without a time = %d, want 0", count)
	}
}

func TestMigrationsKeepTheDueTimeOnAWholeMinuteOfTheDay(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	tests := map[string]string{
		"00:00": "", "23:59": "", "09:00:00": "",
		"24:00": checkViolation, "09:00:30": checkViolation, "12:00:00.5": checkViolation,
	}

	for value, wantCode := range tests {
		_, err := db.Exec(
			`INSERT INTO core.tasks (id, assignee_id, title, status, priority, due_on, created_at, due_time)
			VALUES ($1, $2, 'Call Maria Perez', 'open', 0, '2026-07-30', now(), $3::time)`,
			uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), value,
		)
		if got := sqlStateOf(err); got != wantCode {
			t.Errorf("storing the time %s answered %q (%v), want %q", value, got, err, wantCode)
		}
	}
}

func TestMigrationsStoreContactWithIdentity(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	maria := mustContact(t, "María Pérez")
	insertContact(t, db, maria)

	err := insertIdentity(db, mustIdentity(t, maria.ID, "whatsapp", "184467235@lid"))

	if err != nil {
		t.Fatalf("inserting identity: %v, want nil", err)
	}
}

func TestMigrationsEnforceIdentityUniquenessAcrossContacts(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	maria := mustContact(t, "María Pérez")
	john := mustContact(t, "John Doe")
	insertContact(t, db, maria)
	insertContact(t, db, john)
	if err := insertIdentity(db, mustIdentity(t, maria.ID, "whatsapp", "184467235@lid")); err != nil {
		t.Fatalf("inserting first identity: %v, want nil", err)
	}

	err := insertIdentity(db, mustIdentity(t, john.ID, "whatsapp", "184467235@lid"))

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
		t.Fatalf("inserting duplicate identity: %v, want unique violation %s", err, uniqueViolation)
	}
}

func TestMigrationsRejectIdentityWithoutContact(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	err := insertIdentity(db, mustIdentity(t, uuid.Must(uuid.NewV7()), "whatsapp", "184467235@lid"))

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != foreignKeyViolation {
		t.Fatalf("inserting orphan identity: %v, want foreign key violation %s", err, foreignKeyViolation)
	}
}

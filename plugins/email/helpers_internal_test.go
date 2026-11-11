// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

// errEntropy is the failure of a random source that answers no bytes.
var errEntropy = errors.New("entropy source failed")

// failingReader is a random source that answers errEntropy on every read.
type failingReader struct{}

// Read returns errEntropy instead of any bytes.
func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

// newUnmigratedPool returns a pool over a fresh database holding the core schema alone, beside its address.
func newUnmigratedPool(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	address := testdb.New(t, testdb.Migrator())
	pool, err := pgxpool.New(context.Background(), address)
	if err != nil {
		t.Fatalf("opening a pool over the test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, address
}

// newMigratedPool returns a pool over a fresh database holding the core schema and the email schema.
func newMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := newUnmigratedPool(t)
	if err := migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate() error = %v, want nil", err)
	}
	return pool
}

// sessionTo returns a session of its own on the database at address, closed when the test ends.
func sessionTo(t *testing.T, address string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), address)
	if err != nil {
		t.Fatalf("connecting a session: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// rowReader reads one row on a pool or a session.
type rowReader interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// tableExists reports whether the database behind db holds the table named name.
func tableExists(t *testing.T, db rowReader, name string) bool {
	t.Helper()
	var present bool
	if err := db.QueryRow(t.Context(), "SELECT to_regclass($1) IS NOT NULL", name).Scan(&present); err != nil {
		t.Fatalf("asking for %s: %v", name, err)
	}
	return present
}

// storeWorkspace stores one more workspace, named Acme, and returns its id.
func storeWorkspace(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(), "INSERT INTO core.tenants (id, name) VALUES ($1, 'Acme')", id); err != nil {
		t.Fatalf("storing a second workspace: %v", err)
	}
	return id
}

// beginTx opens a transaction on pool that rolls back when the test ends unless it ended first.
func beginTx(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("beginning a transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

// checkViolation is the code PostgreSQL answers when a row breaks a check constraint.
const checkViolation = "23514"

// foreignKeyViolation is the code PostgreSQL answers when a write or a delete breaks a foreign key.
const foreignKeyViolation = "23503"

// mailReceived is when the mails of the store tests reach their mailbox.
var mailReceived = time.Date(2026, time.November, 2, 9, 0, 0, 0, time.UTC)

// mailTables lists the tables the second migration creates.
var mailTables = []string{
	"plugin_email.threads", "plugin_email.messages", "plugin_email.provider_refs",
	"plugin_email.thread_keys", "plugin_email.message_contacts", "plugin_email.attachments",
}

// execer runs one statement on a pool or inside a transaction.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// pgCode returns the SQLSTATE err carries, empty when it carries none.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// storeContact stores a contact named name in the workspace tenant and returns its id.
func storeContact(t *testing.T, db execer, tenant uuid.UUID, name string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := db.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, $2, now(), $3)",
		id, name, tenant); err != nil {
		t.Fatalf("storing the contact %s: %v", name, err)
	}
	return id
}

// storeEmptyThread stores a thread of the mailbox box that holds no mail yet and returns its id.
func storeEmptyThread(t *testing.T, db execer, tenant uuid.UUID, box mailbox) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := db.Exec(t.Context(), `INSERT INTO plugin_email.threads
		(id, tenant_id, mailbox_id, subject, first_message_at, last_message_at)
		VALUES ($1, $2, $3, 'Your quote', $4, $4)`,
		id, tenant, box.id, mailReceived); err != nil {
		t.Fatalf("storing a thread: %v", err)
	}
	return id
}

// mailRow returns an inbound mail of the thread in the mailbox box with every column set.
func mailRow(box mailbox, thread uuid.UUID) messageRow {
	return messageRow{
		mailboxID:         box.id,
		threadID:          thread,
		providerThreadID:  "file:conversation",
		dedupeKey:         bytes.Repeat([]byte{1}, 32),
		internetMessageID: "quote@example.com",
		inReplyToIDs:      []string{"request@example.com"},
		referenceIDs:      []string{"first@example.com", "request@example.com"},
		direction:         inbound,
		subject:           "Your quote",
		participants:      []participant{{Role: "from", Name: "Maria Perez", Email: "maria.perez@example.com"}},
		sentAt:            mailReceived.Add(-time.Minute),
		receivedAt:        mailReceived,
		bodyText:          "Could you send me a quote?",
		safeHTML:          "<p>Could you send me a quote?</p>",
		snippet:           "Could you send me a quote?",
		bodyState:         bodyWhole,
		skippedParts:      1,
		automation:        message.NotAutomated,
		signed:            true,
		inviteMethod:      "REQUEST",
		rawFileID:         uuid.Must(uuid.NewV7()),
		rawSize:           1200,
		rawSHA256:         bytes.Repeat([]byte{7}, 32),
		rawState:          fileStored,
		storedBytes:       1300,
	}
}

// storeMail stores row in the workspace tenant inside tx and returns its id.
func storeMail(t *testing.T, tx pgx.Tx, tenant uuid.UUID, row messageRow) uuid.UUID {
	t.Helper()
	id, err := insertMessage(t.Context(), tx, tenant, row)
	if err != nil {
		t.Fatalf("insertMessage() error = %v, want nil", err)
	}
	return id
}

// storedRow reads back the columns insertMessage wrote for the mail id names.
func storedRow(t *testing.T, db rowReader, id uuid.UUID) messageRow {
	t.Helper()
	var row messageRow
	var sentAt *time.Time
	var rawFileID *uuid.UUID
	var automation string
	err := db.QueryRow(t.Context(), `SELECT mailbox_id, thread_id, coalesce(provider_thread_id, ''), dedupe_key,
		coalesce(internet_message_id, ''), in_reply_to_ids, reference_ids, direction, subject, participants, sent_at,
		received_at, body_text, safe_html, snippet, body_state, skipped_parts, automation, encrypted, signed, winmail,
		invite_method, raw_file_id, raw_size, raw_sha256, raw_state, stored_bytes
		FROM plugin_email.messages WHERE id = $1`, id).Scan(
		&row.mailboxID, &row.threadID, &row.providerThreadID, &row.dedupeKey, &row.internetMessageID,
		&row.inReplyToIDs, &row.referenceIDs, &row.direction, &row.subject, &row.participants, &sentAt,
		&row.receivedAt, &row.bodyText, &row.safeHTML, &row.snippet, &row.bodyState, &row.skippedParts, &automation,
		&row.encrypted, &row.signed, &row.winmail, &row.inviteMethod, &rawFileID, &row.rawSize, &row.rawSHA256,
		&row.rawState, &row.storedBytes)
	if err != nil {
		t.Fatalf("reading the stored mail: %v", err)
	}
	if sentAt != nil {
		row.sentAt = sentAt.UTC()
	}
	if rawFileID != nil {
		row.rawFileID = *rawFileID
	}
	row.receivedAt = row.receivedAt.UTC()
	row.automation = message.Automation(automation)
	return row
}

// countRows counts the rows a statement over one argument answers.
func countRows(t *testing.T, db rowReader, statement string, arg any) int {
	t.Helper()
	var rows int
	if err := db.QueryRow(t.Context(), statement, arg).Scan(&rows); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	return rows
}

// tenantReferences lists the plugin_email tables whose tenant_id holds a foreign key to core.tenants.
func tenantReferences(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, _ := pool.Query(t.Context(), `SELECT DISTINCT c.conrelid::regclass::text FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
		WHERE c.contype = 'f' AND c.confrelid = 'core.tenants'::regclass AND a.attname = 'tenant_id'
		AND c.connamespace = 'plugin_email'::regnamespace`)
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading the tenant foreign keys: %v", err)
	}
	return tables
}

// newMailbox returns a transaction on a migrated database whose default workspace holds a mailbox, beside the mailbox.
func newMailbox(t *testing.T) (pgx.Tx, mailbox) {
	t.Helper()
	pool := newMigratedPool(t)
	box := storeMailbox(t, pool, sdk.DefaultTenantID)
	return beginTx(t, pool), box
}

// endedTx returns a transaction on a migrated database that already rolled back.
func endedTx(t *testing.T) pgx.Tx {
	t.Helper()
	tx := beginTx(t, newMigratedPool(t))
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatalf("ending the transaction: %v", err)
	}
	return tx
}

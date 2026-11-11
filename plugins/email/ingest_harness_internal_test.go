// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob/memblob"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/sdk"
)

// ingestNow is when the ingest tests judge and record each failure.
var ingestNow = time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)

// testMatchMax is the most addresses one match of the ingest tests takes.
const testMatchMax = 100

// quietSpell is how long a test watches for a call that must not come.
const quietSpell = 200 * time.Millisecond

// callerDeadline is how long a caller with a deadline gives one ingest of a test.
const callerDeadline = 2 * time.Second

// lockWaits counts the other sessions of the same database that wait on a lock.
const lockWaits = "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()" +
	" AND pid <> pg_backend_pid() AND wait_event_type = 'Lock'"

// workspaceRows counts every row the mail of one workspace leaves in the tables of the email plugin.
const workspaceRows = `SELECT
	(SELECT count(*) FROM plugin_email.threads WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.provider_refs WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.thread_keys WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.message_contacts WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.attachments WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.failed_mail WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.workspace_usage WHERE tenant_id = $1)`

// ingestTest is one ingest test: its database, its mailbox, its ingester and the fakes the ingester calls.
type ingestTest struct {
	t         *testing.T
	pool      *pgxpool.Pool
	box       mailbox
	ingester  *ingester
	log       *callLog
	gate      *fakeGate
	matcher   *fakeMatcher
	store     *savingStore
	publisher *steppingPublisher
	logged    *bytes.Buffer
	contacts  map[string]uuid.UUID
}

// newIngestTest returns an ingest test whose default workspace holds the mailbox and knows every known email.
func newIngestTest(t *testing.T, change ...func(*settings)) *ingestTest {
	t.Helper()
	held, err := readSettings(sdk.Deps{})
	if err != nil {
		t.Fatalf("readSettings() error = %v, want nil", err)
	}
	for _, apply := range change {
		apply(&held)
	}
	pool := newMigratedPool(t)
	log := &callLog{}
	logger, written := bufferLogger()
	it := &ingestTest{
		t: t, pool: pool, box: storeMailbox(t, pool, sdk.DefaultTenantID), log: log,
		gate:      &fakeGate{log: log},
		matcher:   &fakeMatcher{log: log, known: map[uuid.UUID]map[string]uuid.UUID{}},
		store:     &savingStore{FileStore: storage.NewFileStore(memblob.OpenBucket(nil), "email", fileCap), log: log},
		publisher: &steppingPublisher{log: log},
		logged:    written,
	}
	it.contacts = it.knowContacts(sdk.DefaultTenantID)
	it.ingester = newIngester(pool, sdk.Deps{
		Matcher: it.matcher, ContactMatchMax: testMatchMax, Events: it.publisher, Logger: logger,
	}, held)
	it.ingester.gate = it.gate
	it.ingester.files.store = it.store
	it.ingester.files.settings = postgres.NewSettingStore(pool, nil).ForPlugin("email")
	it.ingester.now = func() time.Time { return ingestNow }
	return it
}

// knowContacts stores a contact for each known email in the workspace tenant and has the matcher answer them there.
func (it *ingestTest) knowContacts(tenant uuid.UUID) map[string]uuid.UUID {
	it.t.Helper()
	known := map[string]uuid.UUID{}
	for _, email := range knownEmails {
		known[email] = storeContact(it.t, it.pool, tenant, email)
	}
	it.matcher.known[tenant] = known
	return known
}

// incoming returns raw as a mail of the test's mailbox under ref, received at mailReceived.
func (it *ingestTest) incoming(raw []byte, ref string) Incoming {
	return Incoming{MailboxID: it.box.id, Raw: bytes.NewReader(raw), ProviderRef: ref, ReceivedAt: mailReceived}
}

// ingest ingests in for the default workspace and fails the test when it answers an error.
func (it *ingestTest) ingest(in Incoming) Outcome {
	it.t.Helper()
	outcome, err := it.ingester.ingest(it.t.Context(), sdk.DefaultTenantID, in)
	if err != nil {
		it.t.Fatalf("ingest() error = %v, want nil", err)
	}
	return outcome
}

// mailTitled returns the id of the one stored mail with subject, failing the test unless exactly one holds it.
func (it *ingestTest) mailTitled(subject string) uuid.UUID {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(), "SELECT id FROM plugin_email.messages WHERE subject = $1", subject)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil || len(ids) != 1 {
		it.t.Fatalf("the mails titled %q are %v, %v, want exactly one", subject, ids, err)
	}
	return ids[0]
}

// mails counts the stored mails of the default workspace.
func (it *ingestTest) mails() int {
	it.t.Helper()
	return countRows(it.t, it.pool, "SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1",
		sdk.DefaultTenantID)
}

// rows counts every row the mail of the workspace tenant leaves.
func (it *ingestTest) rows(tenant uuid.UUID) int {
	it.t.Helper()
	return countRows(it.t, it.pool, workspaceRows, tenant)
}

// links returns the contacts the mail id is linked to, in the order of their ids.
func (it *ingestTest) links(id uuid.UUID) []uuid.UUID {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(),
		"SELECT contact_id FROM plugin_email.message_contacts WHERE message_id = $1 ORDER BY contact_id", id)
	contacts, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		it.t.Fatalf("reading the links of %v: %v", id, err)
	}
	return contacts
}

// knownContacts returns the contacts of the default workspace each email names, in the order of their ids.
func (it *ingestTest) knownContacts(emails ...string) []uuid.UUID {
	contacts := make([]uuid.UUID, 0, len(emails))
	for _, email := range emails {
		contacts = append(contacts, it.contacts[email])
	}
	return slices.SortedFunc(slices.Values(contacts), func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
}

// attachmentsOf returns the attachment rows of the mail id in position order.
func (it *ingestTest) attachmentsOf(id uuid.UUID) []attachmentRow {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(), `SELECT position, filename, content_type, size, sha256, inline,
		content_id, file_id, state FROM plugin_email.attachments WHERE message_id = $1 ORDER BY position`, id)
	held, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (attachmentRow, error) {
		var file attachmentRow
		var fileID *uuid.UUID
		err := row.Scan(&file.position, &file.filename, &file.contentType, &file.size, &file.sha256, &file.inline,
			&file.contentID, &fileID, &file.state)
		if fileID != nil {
			file.fileID = *fileID
		}
		return file, err
	})
	if err != nil {
		it.t.Fatalf("reading the attachments of %v: %v", id, err)
	}
	return held
}

// events returns every event the publisher was handed.
func (it *ingestTest) events() []published {
	it.publisher.mu.Lock()
	defer it.publisher.mu.Unlock()
	return slices.Clone(it.publisher.events)
}

// queueStates returns the state of every queue row of the default workspace, in order.
func (it *ingestTest) queueStates() []string {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(),
		"SELECT state FROM plugin_email.pending_files WHERE tenant_id = $1 ORDER BY state", sdk.DefaultTenantID)
	states, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		it.t.Fatalf("reading the queue: %v", err)
	}
	return states
}

// exec runs each statement on the test's database in order.
func (it *ingestTest) exec(statements ...string) {
	it.t.Helper()
	for _, statement := range statements {
		if _, err := it.pool.Exec(it.t.Context(), statement); err != nil {
			it.t.Fatalf("running %q: %v", statement, err)
		}
	}
}

// refuseNewMail makes every new mail row fail with the SQLSTATE code, its message naming an address.
func (it *ingestTest) refuseNewMail(code string) {
	it.t.Helper()
	it.exec(`CREATE FUNCTION refuse_new_mail() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'refused the mail of maria.perez@example.com' USING ERRCODE = '`+code+`'; END $$`,
		`CREATE TRIGGER refuse_new_mail BEFORE INSERT ON plugin_email.messages
		FOR EACH ROW EXECUTE FUNCTION refuse_new_mail()`)
}

// holdMailbox locks the row of the test's mailbox in a transaction of its own, rolled back at cleanup.
func (it *ingestTest) holdMailbox() pgx.Tx {
	it.t.Helper()
	tx := beginTx(it.t, it.pool)
	if _, err := tx.Exec(it.t.Context(), "SELECT id FROM plugin_email.mailboxes WHERE id = $1 FOR UPDATE",
		it.box.id); err != nil {
		it.t.Fatalf("locking the mailbox: %v", err)
	}
	return tx
}

// awaitLockWaits returns once count other sessions of the test's database wait on a lock.
func (it *ingestTest) awaitLockWaits(count int) {
	it.t.Helper()
	started := time.Now()
	for {
		var waiting int
		if err := it.pool.QueryRow(it.t.Context(), lockWaits).Scan(&waiting); err != nil {
			it.t.Fatalf("counting the sessions that wait on a lock: %v", err)
		}
		if waiting >= count {
			return
		}
		if time.Since(started) > rivalBudget {
			it.t.Fatalf("%d sessions wait on a lock after %v, want %d", waiting, rivalBudget, count)
		}
		time.Sleep(rivalPoll)
	}
}

// awaitStep returns once the fakes saw step count times.
func (it *ingestTest) awaitStep(step string, count int) {
	it.t.Helper()
	started := time.Now()
	for {
		seen := 0
		for _, called := range it.log.steps() {
			if called == step {
				seen++
			}
		}
		if seen >= count {
			return
		}
		if time.Since(started) > rivalBudget {
			it.t.Fatalf("the fakes saw %d %s calls after %v, want %d", seen, step, rivalBudget, count)
		}
		time.Sleep(rivalPoll)
	}
}

// prepareMail reads raw under ref and keeps its files as ingest does before its transaction.
func (it *ingestTest) prepareMail(raw []byte, ref string, sentCopy bool) (parsedMail, messageRow, []keptFile) {
	it.t.Helper()
	in := it.incoming(raw, ref)
	in.SentCopy = sentCopy
	mail, err := it.ingester.readIncoming(it.t.Context(), it.box, in)
	if err != nil {
		it.t.Fatalf("readIncoming() error = %v, want nil", err)
	}
	kept, err := it.ingester.files.keepFiles(it.t.Context(), sdk.DefaultTenantID,
		it.ingester.files.filesOf(mail.message, true), fileStored)
	if err != nil {
		it.t.Fatalf("keepFiles() error = %v, want nil", err)
	}
	return mail, withFiles(it.ingester.rowOf(mail), kept), kept
}

// sweep runs one sweep of the queue over the test's database and file store.
func (it *ingestTest) sweep() {
	queueSweeper := &sweeper{pool: it.pool, store: it.store, logger: slog.New(slog.DiscardHandler),
		pace: sweepSettings{interval: time.Hour, batch: 10, budget: time.Minute, orphanAge: orphanAge}}
	queueSweeper.sweep(it.t.Context())
}

// wantFilesKept fails the test unless the store keeps every byte of each file of kept.
func (it *ingestTest) wantFilesKept(kept []keptFile) {
	it.t.Helper()
	for _, file := range kept {
		if held := storedBytes(it.t, it.store, sdk.DefaultTenantID, file.fileID()); len(held) != int(file.size) {
			it.t.Errorf("the store keeps %d bytes of %v, want its %d", len(held), file.fileID(), file.size)
		}
	}
}

// textBytes counts the text row keeps: its body, its safe HTML, its snippet and its participants.
func textBytes(t *testing.T, row messageRow) int64 {
	t.Helper()
	participants, err := json.Marshal(row.participants)
	if err != nil {
		t.Fatalf("encoding the participants: %v", err)
	}
	return int64(len(row.bodyText) + len(row.safeHTML) + len(row.snippet) + len(participants))
}

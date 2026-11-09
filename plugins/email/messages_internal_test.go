// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

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

func TestMigrateCreatesTheMailTables(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	for _, table := range mailTables {
		if !tableExists(t, pool, table) {
			t.Errorf("migrate() left out %s, want it created", table)
		}
	}
}

func TestMigrateIndexesTheFilesAStoredMailHolds(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	for name, want := range map[string]string{
		"messages_raw_file_id_idx": "ON plugin_email.messages USING btree (raw_file_id, tenant_id) " +
			"WHERE (raw_file_id IS NOT NULL)",
		"attachments_file_id_idx": "ON plugin_email.attachments USING btree (file_id, tenant_id) " +
			"WHERE (file_id IS NOT NULL)",
	} {
		var definition string
		err := pool.QueryRow(t.Context(),
			"SELECT indexdef FROM pg_indexes WHERE schemaname = 'plugin_email' AND indexname = $1", name,
		).Scan(&definition)

		if err != nil || !strings.HasSuffix(definition, want) {
			t.Errorf("the index %s = %q, %v, want it to end %q", name, definition, err, want)
		}
	}
}

func TestInsertMessageStoresEveryColumn(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))

	id, err := insertMessage(t.Context(), tx, sdk.DefaultTenantID, row)

	if err != nil {
		t.Fatalf("insertMessage() error = %v, want nil", err)
	}
	if id == uuid.Nil || id.Version() != 7 {
		t.Errorf("insertMessage() id = %v, want a new UUIDv7", id)
	}
	if got := storedRow(t, tx, id); !reflect.DeepEqual(got, row) {
		t.Errorf("the stored mail = %+v, want %+v", got, row)
	}
}

func TestInsertMessageStoresNoSentTimeForAZeroDate(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	row.sentAt = time.Time{}

	id := storeMail(t, tx, sdk.DefaultTenantID, row)

	var missing bool
	if err := tx.QueryRow(t.Context(),
		"SELECT sent_at IS NULL FROM plugin_email.messages WHERE id = $1", id).Scan(&missing); err != nil {
		t.Fatalf("reading sent_at: %v", err)
	}
	if !missing {
		t.Error("sent_at holds a time for a mail with a zero Date, want NULL")
	}
}

func TestInsertMessageStoresNoTextIDOrFileAsNull(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	row.providerThreadID, row.internetMessageID, row.dedupeKey = "", "", nil
	row.inReplyToIDs, row.referenceIDs, row.rawFileID, row.rawSHA256 = nil, nil, uuid.Nil, nil
	row.rawState = rawNone

	id := storeMail(t, tx, sdk.DefaultTenantID, row)

	var nulls bool
	var inReplyTo, references []string
	if err := tx.QueryRow(t.Context(), `SELECT provider_thread_id IS NULL AND internet_message_id IS NULL
		AND dedupe_key IS NULL AND raw_file_id IS NULL AND raw_sha256 IS NULL, in_reply_to_ids, reference_ids
		FROM plugin_email.messages WHERE id = $1`, id).Scan(&nulls, &inReplyTo, &references); err != nil {
		t.Fatalf("reading the empty columns: %v", err)
	}
	if !nulls {
		t.Error("an empty id, key or file is stored as a value, want NULL")
	}
	if inReplyTo == nil || len(inReplyTo) != 0 || references == nil || len(references) != 0 {
		t.Errorf("in_reply_to_ids = %#v, reference_ids = %#v, want two empty lists", inReplyTo, references)
	}
}

func TestInsertMessageKeepsTheParticipantsAsAListWithRoles(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	row.participants = []participant{
		{Role: "from", Name: "Maria Perez", Email: "maria.perez@example.com"},
		{Role: "cc", Email: "ada@example.com"},
	}

	id := storeMail(t, tx, sdk.DefaultTenantID, row)

	var kind, first, second string
	if err := tx.QueryRow(t.Context(), `SELECT jsonb_typeof(participants), participants->0->>'role',
		participants->1->>'role' FROM plugin_email.messages WHERE id = $1`, id).Scan(&kind, &first, &second); err != nil {
		t.Fatalf("reading the participants: %v", err)
	}
	if kind != "array" || first != "from" || second != "cc" {
		t.Errorf("participants = %s with roles %q and %q, want an array with the roles from and cc", kind, first, second)
	}
	if got := storedRow(t, tx, id).participants; !reflect.DeepEqual(got, row.participants) {
		t.Errorf("the stored participants = %+v, want %+v", got, row.participants)
	}
}

func TestTheMessagesTableTakesTheSevenAutomationValuesOnly(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeEmptyThread(t, tx, sdk.DefaultTenantID, box)
	for _, automation := range []message.Automation{
		message.NotAutomated, message.AutoGenerated, message.AutoReply, message.ListMail,
		message.Bounce, message.AbuseReport, message.ReadReceipt,
	} {
		row := mailRow(box, thread)
		row.dedupeKey, row.automation = nil, automation
		storeMail(t, tx, sdk.DefaultTenantID, row)
	}

	row := mailRow(box, thread)
	row.automation = "robot"
	_, err := insertMessage(t.Context(), tx, sdk.DefaultTenantID, row)

	if code := pgCode(err); code != checkViolation {
		t.Errorf("insertMessage() of the automation robot error = %v, want the check to refuse it", err)
	}
}

func TestTheMessagesTableTakesTheEightCalendarMethodsOrNone(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeEmptyThread(t, tx, sdk.DefaultTenantID, box)
	for _, method := range []string{
		"", "PUBLISH", "REQUEST", "REPLY", "ADD", "CANCEL", "REFRESH", "COUNTER", "DECLINECOUNTER",
	} {
		row := mailRow(box, thread)
		row.dedupeKey, row.inviteMethod = nil, method
		storeMail(t, tx, sdk.DefaultTenantID, row)
	}

	row := mailRow(box, thread)
	row.inviteMethod = "ANNOUNCE"
	_, err := insertMessage(t.Context(), tx, sdk.DefaultTenantID, row)

	if code := pgCode(err); code != checkViolation {
		t.Errorf("insertMessage() of the method ANNOUNCE error = %v, want the check to refuse it", err)
	}
}

func TestTheMessagesTableTakesTheFourRawStatesOnly(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeEmptyThread(t, tx, sdk.DefaultTenantID, box)
	for _, state := range []string{fileStored, fileTooLarge, fileOverLimit, rawNone} {
		row := mailRow(box, thread)
		row.dedupeKey, row.rawState = nil, state
		storeMail(t, tx, sdk.DefaultTenantID, row)
	}

	row := mailRow(box, thread)
	row.rawState = "deleted"
	_, err := insertMessage(t.Context(), tx, sdk.DefaultTenantID, row)

	if code := pgCode(err); code != checkViolation {
		t.Errorf("insertMessage() of the raw state deleted error = %v, want the check to refuse it", err)
	}
}

func TestTheMessagesTableRefusesAnUnknownDirectionOrBodyState(t *testing.T) {
	t.Parallel()

	for name, change := range map[string]func(*messageRow){
		"direction":  func(row *messageRow) { row.direction = "forwarded" },
		"body state": func(row *messageRow) { row.bodyState = "partial" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tx, box := newMailbox(t)
			thread := storeEmptyThread(t, tx, sdk.DefaultTenantID, box)
			for _, state := range []string{bodyWhole, bodyCut, bodyHeadersOnly} {
				row := mailRow(box, thread)
				row.dedupeKey, row.bodyState, row.direction = nil, state, outbound
				storeMail(t, tx, sdk.DefaultTenantID, row)
			}
			row := mailRow(box, thread)
			change(&row)

			_, err := insertMessage(t.Context(), tx, sdk.DefaultTenantID, row)

			if code := pgCode(err); code != checkViolation {
				t.Errorf("insertMessage() of an unknown %s error = %v, want the check to refuse it", name, err)
			}
		})
	}
}

func TestTheAttachmentsTableTakesTheThreeFileStatesOnly(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))
	var files []attachmentRow
	for position, state := range []string{fileStored, fileTooLarge, fileOverLimit} {
		files = append(files, attachmentRow{position: position, filename: "quote.pdf", contentType: "application/pdf",
			size: 10, sha256: bytes.Repeat([]byte{2}, 32), state: state})
	}
	if err := insertAttachments(t.Context(), tx, sdk.DefaultTenantID, id, files); err != nil {
		t.Fatalf("insertAttachments() of the three file states error = %v, want nil", err)
	}

	err := insertAttachments(t.Context(), tx, sdk.DefaultTenantID, id, []attachmentRow{{
		filename: "quote.pdf", contentType: "application/pdf", sha256: []byte{2}, state: "deleted",
	}})

	if code := pgCode(err); code != checkViolation {
		t.Errorf("insertAttachments() of the state deleted error = %v, want the check to refuse it", err)
	}
}

func TestDeletingAContactWhoHoldsMailFails(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	box := storeMailbox(t, pool, sdk.DefaultTenantID)
	contact := storeContact(t, pool, sdk.DefaultTenantID, "Contact who holds mail")
	tx := beginTx(t, pool)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))
	if _, err := linkContacts(t.Context(), tx, sdk.DefaultTenantID, id, []uuid.UUID{contact}); err != nil {
		t.Fatalf("linkContacts() error = %v, want nil", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("committing the mail: %v", err)
	}

	_, err := pool.Exec(t.Context(), "DELETE FROM core.contacts WHERE tenant_id = $1 AND id = $2",
		sdk.DefaultTenantID, contact)

	if code := pgCode(err); code != foreignKeyViolation {
		t.Errorf("deleting a contact who holds mail error = %v, want the link to refuse it", err)
	}
	if contacts := countRows(t, pool, "SELECT count(*) FROM core.contacts WHERE id = $1", contact); contacts != 1 {
		t.Errorf("the contact is gone, want it kept")
	}
	if links := countRows(t, pool,
		"SELECT count(*) FROM plugin_email.message_contacts WHERE contact_id = $1", contact); links != 1 {
		t.Errorf("the contact holds %d links, want its one link kept", links)
	}
}

func TestInsertMessageAnswersAlreadyStoredForAKeyTheMailboxHolds(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	storeMail(t, tx, sdk.DefaultTenantID, row)

	id, err := insertMessage(t.Context(), tx, sdk.DefaultTenantID, row)

	if !errors.Is(err, errAlreadyStored) || id != uuid.Nil {
		t.Errorf("insertMessage() of a held dedupe key = %v, %v, want %v", id, err, errAlreadyStored)
	}
	if mails := countRows(t, tx, "SELECT count(*) FROM plugin_email.messages WHERE mailbox_id = $1", box.id); mails != 1 {
		t.Errorf("the mailbox holds %d mails, want 1", mails)
	}
}

func TestInsertMessageStoresTwoMailsThatHoldNoDedupeKey(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	row.dedupeKey = nil

	first := storeMail(t, tx, sdk.DefaultTenantID, row)
	second := storeMail(t, tx, sdk.DefaultTenantID, row)

	if first == second {
		t.Errorf("two mails with no dedupe key share the id %v, want two rows", first)
	}
	if mails := countRows(t, tx, "SELECT count(*) FROM plugin_email.messages WHERE mailbox_id = $1", box.id); mails != 2 {
		t.Errorf("the mailbox holds %d mails, want 2", mails)
	}
}

func TestInsertMessageReportsAnIDItCannotMake(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	_, err := insertMessage(t.Context(), nil, sdk.DefaultTenantID, messageRow{})

	if !errors.Is(err, errEntropy) {
		t.Errorf("insertMessage() error = %v, want %v", err, errEntropy)
	}
}

func TestAddProviderRefKeepsTwoReferencesOfOneMail(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))

	for _, ref := range []providerRef{
		{mailboxID: box.id, ref: "file:inbox", messageID: id},
		{mailboxID: box.id, ref: "file:sent", messageID: id, sentCopy: true},
	} {
		if err := addProviderRef(t.Context(), tx, sdk.DefaultTenantID, ref); err != nil {
			t.Fatalf("addProviderRef(%s) error = %v, want nil", ref.ref, err)
		}
	}

	if refs := countRows(t, tx, "SELECT count(*) FROM plugin_email.provider_refs WHERE message_id = $1", id); refs != 2 {
		t.Errorf("the mail holds %d provider references, want 2", refs)
	}
	sentCopies := countRows(t, tx,
		"SELECT count(*) FROM plugin_email.provider_refs WHERE message_id = $1 AND sent_copy", id)
	if sentCopies != 1 {
		t.Errorf("the mail holds %d Sent copies, want 1", sentCopies)
	}
}

func TestAddProviderRefLeavesAReferenceTheMailboxHoldsAsItIs(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeEmptyThread(t, tx, sdk.DefaultTenantID, box)
	held := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, thread))
	other := mailRow(box, thread)
	other.dedupeKey = nil
	second := storeMail(t, tx, sdk.DefaultTenantID, other)
	if err := addProviderRef(t.Context(), tx, sdk.DefaultTenantID,
		providerRef{mailboxID: box.id, ref: "file:inbox", messageID: held}); err != nil {
		t.Fatalf("addProviderRef() error = %v, want nil", err)
	}

	err := addProviderRef(t.Context(), tx, sdk.DefaultTenantID,
		providerRef{mailboxID: box.id, ref: "file:inbox", messageID: second, sentCopy: true})

	if err != nil {
		t.Fatalf("addProviderRef() of a held reference error = %v, want nil", err)
	}
	got, err := readMessageIDByProviderRef(t.Context(), tx, sdk.DefaultTenantID, box.id, "file:inbox")
	if err != nil || got != held {
		t.Errorf("the reference leads to %v, %v, want the mail that held it first, %v", got, err, held)
	}
}

func TestReadMessageIDByProviderRefAnswersTheMailItNames(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))
	if err := addProviderRef(t.Context(), tx, sdk.DefaultTenantID,
		providerRef{mailboxID: box.id, ref: "file:inbox", messageID: id}); err != nil {
		t.Fatalf("addProviderRef() error = %v, want nil", err)
	}

	got, err := readMessageIDByProviderRef(t.Context(), tx, sdk.DefaultTenantID, box.id, "file:inbox")

	if err != nil || got != id {
		t.Errorf("readMessageIDByProviderRef() = %v, %v, want %v", got, err, id)
	}
}

func TestReadMessageIDByProviderRefReportsAReferenceNoMailOfTheWorkspaceHolds(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	second := storeWorkspace(t, pool)
	box := storeMailbox(t, pool, second)
	tx := beginTx(t, pool)
	id := storeMail(t, tx, second, mailRow(box, storeEmptyThread(t, tx, second, box)))
	if err := addProviderRef(t.Context(), tx, second,
		providerRef{mailboxID: box.id, ref: "file:inbox", messageID: id}); err != nil {
		t.Fatalf("addProviderRef() error = %v, want nil", err)
	}

	for _, unheld := range []struct {
		name   string
		tenant uuid.UUID
		ref    string
	}{
		{name: "another workspace's", tenant: sdk.DefaultTenantID, ref: "file:inbox"},
		{name: "an unknown", tenant: second, ref: "file:unknown"},
	} {
		_, err := readMessageIDByProviderRef(t.Context(), tx, unheld.tenant, box.id, unheld.ref)

		if !errors.Is(err, pgx.ErrNoRows) || !strings.HasPrefix(err.Error(), "email: read the mail of a provider ref: ") {
			t.Errorf("readMessageIDByProviderRef() of %s reference error = %v, want no rows named", unheld.name, err)
		}
	}
}

func TestReadMessageIDByDedupeKeyAnswersTheMailItNames(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	id := storeMail(t, tx, sdk.DefaultTenantID, row)

	got, err := readMessageIDByDedupeKey(t.Context(), tx, sdk.DefaultTenantID, box.id, row.dedupeKey)

	if err != nil || got != id {
		t.Errorf("readMessageIDByDedupeKey() = %v, %v, want %v", got, err, id)
	}
}

func TestReadMessageIDByDedupeKeyReportsAKeyNoMailHolds(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))

	_, err := readMessageIDByDedupeKey(t.Context(), tx, sdk.DefaultTenantID, box.id, bytes.Repeat([]byte{9}, 32))

	if !errors.Is(err, pgx.ErrNoRows) || !strings.HasPrefix(err.Error(), "email: read the mail of a dedupe key: ") {
		t.Errorf("readMessageIDByDedupeKey() of an unknown key error = %v, want no rows named", err)
	}
}

func TestLinkContactsCountsOnlyTheNewLinks(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))
	sender := storeContact(t, tx, sdk.DefaultTenantID, "Sender contact")
	copied := storeContact(t, tx, sdk.DefaultTenantID, "Copied contact")
	later := storeContact(t, tx, sdk.DefaultTenantID, "Contact named by a later copy")

	first, err := linkContacts(t.Context(), tx, sdk.DefaultTenantID, id, []uuid.UUID{sender, copied})
	if err != nil || first != 2 {
		t.Fatalf("linkContacts() of two contacts = %d, %v, want 2 new links", first, err)
	}
	second, err := linkContacts(t.Context(), tx, sdk.DefaultTenantID, id, []uuid.UUID{copied, later})

	if err != nil || second != 1 {
		t.Errorf("linkContacts() of one held and one new contact = %d, %v, want 1 new link", second, err)
	}
	if links := countRows(t, tx,
		"SELECT count(*) FROM plugin_email.message_contacts WHERE message_id = $1", id); links != 3 {
		t.Errorf("the mail holds %d links, want 3", links)
	}
}

func TestLinkContactsLeavesOutAContactOfAnotherWorkspace(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	box := storeMailbox(t, pool, sdk.DefaultTenantID)
	foreign := storeContact(t, pool, storeWorkspace(t, pool), "Contact of another workspace")
	tx := beginTx(t, pool)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))

	linked, err := linkContacts(t.Context(), tx, sdk.DefaultTenantID, id, []uuid.UUID{foreign})

	if err != nil || linked != 0 {
		t.Errorf("linkContacts() of another workspace's contact = %d, %v, want no link", linked, err)
	}
}

func TestInsertAttachmentsStoresEachFileWithOrWithoutItsBytes(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))
	files := []attachmentRow{
		{position: 0, filename: "quote.pdf", contentType: "application/pdf", size: 3000,
			sha256: bytes.Repeat([]byte{3}, 32), fileID: uuid.Must(uuid.NewV7()), state: fileStored},
		{position: 1, filename: "photo.png", contentType: "image/png", size: 9000, sha256: bytes.Repeat([]byte{4}, 32),
			inline: true, contentID: "a+b@example.com", state: fileTooLarge},
	}

	err := insertAttachments(t.Context(), tx, sdk.DefaultTenantID, id, files)

	if err != nil {
		t.Fatalf("insertAttachments() error = %v, want nil", err)
	}
	rows, _ := tx.Query(t.Context(), `SELECT position, filename, content_type, size, sha256, inline, content_id,
		file_id, state FROM plugin_email.attachments WHERE message_id = $1 ORDER BY position`, id)
	stored, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (attachmentRow, error) {
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
		t.Fatalf("reading the attachments: %v", err)
	}
	if !reflect.DeepEqual(stored, files) {
		t.Errorf("the stored attachments = %+v, want %+v", stored, files)
	}
}

func TestInsertAttachmentsStoresNothingForAMailWithNoFile(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))

	if err := insertAttachments(t.Context(), tx, sdk.DefaultTenantID, id, nil); err != nil {
		t.Fatalf("insertAttachments() of no file error = %v, want nil", err)
	}
	if files := countRows(t, tx, "SELECT count(*) FROM plugin_email.attachments WHERE message_id = $1", id); files != 0 {
		t.Errorf("the mail holds %d attachments, want none", files)
	}
}

func TestInsertAttachmentsReportsAnIDItCannotMake(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	err := insertAttachments(t.Context(), nil, sdk.DefaultTenantID, uuid.Nil, []attachmentRow{{}})

	if !errors.Is(err, errEntropy) {
		t.Errorf("insertAttachments() error = %v, want %v", err, errEntropy)
	}
}

func TestAddParticipantsAddsOnlyTheAddressesTheMailLacks(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	id := storeMail(t, tx, sdk.DefaultTenantID, row)
	copied := participant{Role: "cc", Email: "ada@example.com"}

	err := addParticipants(t.Context(), tx, sdk.DefaultTenantID, id, []participant{row.participants[0], copied}, 500)

	if err != nil {
		t.Fatalf("addParticipants() error = %v, want nil", err)
	}
	want := []participant{row.participants[0], copied}
	if got := storedRow(t, tx, id).participants; !reflect.DeepEqual(got, want) {
		t.Errorf("the participants = %+v, want %+v", got, want)
	}
}

func TestAddParticipantsStopsAtTheRecipientCap(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	id := storeMail(t, tx, sdk.DefaultTenantID, row)
	copied := []participant{{Role: "cc", Email: "ada@example.com"}, {Role: "cc", Email: "sales@example.com"}}

	if err := addParticipants(t.Context(), tx, sdk.DefaultTenantID, id, copied, 2); err != nil {
		t.Fatalf("addParticipants() error = %v, want nil", err)
	}

	want := []participant{row.participants[0], copied[0]}
	if got := storedRow(t, tx, id).participants; !reflect.DeepEqual(got, want) {
		t.Errorf("the participants under a cap of 2 = %+v, want %+v", got, want)
	}
}

func TestTurnOutboundTurnsAnInboundMailOutbound(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeEmptyThread(t, tx, sdk.DefaultTenantID, box)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, thread))
	other := mailRow(box, thread)
	other.dedupeKey = nil
	untouched := storeMail(t, tx, sdk.DefaultTenantID, other)

	if err := turnOutbound(t.Context(), tx, sdk.DefaultTenantID, id); err != nil {
		t.Fatalf("turnOutbound() error = %v, want nil", err)
	}

	if direction := storedRow(t, tx, id).direction; direction != outbound {
		t.Errorf("the turned mail is %s, want %s", direction, outbound)
	}
	if direction := storedRow(t, tx, untouched).direction; direction != inbound {
		t.Errorf("another mail is %s, want it left %s", direction, inbound)
	}
}

func TestMessageStatementsReportAnEndedTransaction(t *testing.T) {
	t.Parallel()

	tx := endedTx(t)
	ctx, id := t.Context(), uuid.Must(uuid.NewV7())
	for prefix, call := range map[string]func() error{
		"email: insert message: ": func() error {
			_, err := insertMessage(ctx, tx, sdk.DefaultTenantID, messageRow{})
			return err
		},
		"email: add provider ref: ": func() error {
			return addProviderRef(ctx, tx, sdk.DefaultTenantID, providerRef{})
		},
		"email: read the mail of a provider ref: ": func() error {
			_, err := readMessageIDByProviderRef(ctx, tx, sdk.DefaultTenantID, id, "file:inbox")
			return err
		},
		"email: read the mail of a dedupe key: ": func() error {
			_, err := readMessageIDByDedupeKey(ctx, tx, sdk.DefaultTenantID, id, []byte{1})
			return err
		},
		"email: link contacts: ": func() error {
			_, err := linkContacts(ctx, tx, sdk.DefaultTenantID, id, []uuid.UUID{id})
			return err
		},
		"email: insert attachments: ": func() error {
			return insertAttachments(ctx, tx, sdk.DefaultTenantID, id, nil)
		},
		"email: add participants: ": func() error {
			return addParticipants(ctx, tx, sdk.DefaultTenantID, id, nil, 1)
		},
		"email: turn outbound: ": func() error {
			return turnOutbound(ctx, tx, sdk.DefaultTenantID, id)
		},
	} {
		err := call()

		if !errors.Is(err, pgx.ErrTxClosed) || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("a statement on an ended transaction error = %v, want %q naming %v", err, prefix, pgx.ErrTxClosed)
		}
	}
}

func TestEveryMailTableReferencesItsWorkspace(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	referencing := tenantReferences(t, pool)

	for _, table := range mailTables {
		if !slices.Contains(referencing, table) {
			t.Errorf("%s.tenant_id holds no foreign key to core.tenants, want it to reference the workspace", table)
		}
	}
}

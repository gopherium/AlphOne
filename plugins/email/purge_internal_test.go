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

	"github.com/gopherium/alphone/sdk"
)

// testSyncWindow is the sync window the delete tests judge a whole delete by, the 30 days of the default.
const testSyncWindow = 720 * time.Hour

// syncedRef is a reference a mailbox sync would write, one that neither email:ingest nor the seed writes.
const syncedRef = "provider:mail"

// mailRowsOf counts the threads, mails, references, keys, links and files one workspace keeps.
const mailRowsOf = `SELECT
	(SELECT count(*) FROM plugin_email.threads WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.provider_refs WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.thread_keys WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.message_contacts WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.attachments WHERE tenant_id = $1)`

// guardRow is one key of a deleted mail a workspace keeps, beside the mail it named and when that mail went.
type guardRow struct {
	mailboxID uuid.UUID
	key       []byte
	messageID uuid.UUID
	deletedAt time.Time
}

// quoteReplyMail returns a reply of Maria Perez to the quote she asked for, under the id and with the text given.
func quoteReplyMail(id, text string) []byte {
	return rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: Re: Your quote",
		"Date: "+quoteDate, "Message-ID: <"+id+">", "In-Reply-To: <quote@example.com>",
		"References: <quote@example.com>", "Content-Type: text/plain", "", text)
}

// ingestAt stores raw under ref as received the given time after mailReceived, failing unless it is stored.
func (it *ingestTest) ingestAt(raw []byte, ref string, after time.Duration) {
	it.t.Helper()
	in := it.incoming(raw, ref)
	in.ReceivedAt = mailReceived.Add(after)
	if outcome := it.ingest(in); outcome != outcomeStored {
		it.t.Fatalf("ingest() under %s = %q, want %q", ref, outcome, outcomeStored)
	}
}

// mailByMessageID returns the stored mail of the default workspace whose Message-ID is id.
func (it *ingestTest) mailByMessageID(id string) uuid.UUID {
	it.t.Helper()
	var mail uuid.UUID
	if err := it.pool.QueryRow(it.t.Context(), `SELECT id FROM plugin_email.messages
		WHERE tenant_id = $1 AND internet_message_id = $2`, sdk.DefaultTenantID, id).Scan(&mail); err != nil {
		it.t.Fatalf("reading the mail %s: %v", id, err)
	}
	return mail
}

// threadOfThree stores the quote, a reply an hour later under replyRef and a second reply an hour after that.
func (it *ingestTest) threadOfThree(replyRef string) (uuid.UUID, []uuid.UUID) {
	it.t.Helper()
	it.ingestAt(lampMail(), "file:quote", 0)
	it.ingestAt(quoteReplyMail("quote-reply@example.com", "Thank you, the quote is fine."), replyRef, time.Hour)
	it.ingestAt(quoteReplyMail("quote-second-reply@example.com", "Could it come on Friday?"), "file:second-reply",
		2*time.Hour)
	mails := []uuid.UUID{
		it.mailByMessageID("quote@example.com"), it.mailByMessageID("quote-reply@example.com"),
		it.mailByMessageID("quote-second-reply@example.com"),
	}
	thread := storedRow(it.t, it.pool, mails[0]).threadID
	for _, mail := range mails[1:] {
		if joined := storedRow(it.t, it.pool, mail).threadID; joined != thread {
			it.t.Fatalf("the reply %v sits in the thread %v, want the quote's %v", mail, joined, thread)
		}
	}
	return thread, mails
}

// threadOf returns the thread the stored mail id sits in.
func (it *ingestTest) threadOf(id uuid.UUID) uuid.UUID {
	it.t.Helper()
	return storedRow(it.t, it.pool, id).threadID
}

// attachmentIDs returns the ids of the files of the mail id in position order.
func (it *ingestTest) attachmentIDs(id uuid.UUID) []uuid.UUID {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(),
		"SELECT id FROM plugin_email.attachments WHERE message_id = $1 ORDER BY position", id)
	files, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		it.t.Fatalf("reading the files of %v: %v", id, err)
	}
	return files
}

// deleteWhole deletes the mail of a thread of the default workspace message names, every mail when it is nil.
func (it *ingestTest) deleteWhole(thread uuid.UUID, message *uuid.UUID, at time.Time) (threadDeletion, error) {
	return deleteThreadMail(it.t.Context(), it.pool, sdk.DefaultTenantID,
		threadDelete{thread: thread, message: message, at: at, window: testSyncWindow})
}

// freeFiles deletes the files of a mail of the default workspace request names.
func (it *ingestTest) freeFiles(request filesDelete) (filesDeletion, error) {
	return deleteFiles(it.t.Context(), it.pool, sdk.DefaultTenantID, request)
}

// deleteInTx deletes the mail of ids in the default workspace in one transaction under the mailbox lock.
func (it *ingestTest) deleteInTx(ids ...uuid.UUID) (int, error) {
	it.t.Helper()
	tx := beginTx(it.t, it.pool)
	if err := lockMailbox(it.t.Context(), tx, sdk.DefaultTenantID, it.box.id); err != nil {
		it.t.Fatalf("lockMailbox() error = %v, want nil", err)
	}
	deleted, err := deleteMessages(it.t.Context(), tx, sdk.DefaultTenantID, ids, ingestNow)
	if err != nil {
		_ = tx.Rollback(it.t.Context())
		return deleted, err
	}
	if err := tx.Commit(it.t.Context()); err != nil {
		it.t.Fatalf("committing the delete: %v", err)
	}
	return deleted, nil
}

// guardRows returns every key of deleted mail the workspace tenant keeps, in key order.
func guardRows(t *testing.T, db querier, tenant uuid.UUID) []guardRow {
	t.Helper()
	rows, _ := db.Query(t.Context(), `SELECT mailbox_id, dedupe_key, deleted_message_id, deleted_at
		FROM plugin_email.deleted_dedupe_keys WHERE tenant_id = $1 ORDER BY dedupe_key`, tenant)
	held, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (guardRow, error) {
		var guard guardRow
		err := row.Scan(&guard.mailboxID, &guard.key, &guard.messageID, &guard.deletedAt)
		guard.deletedAt = guard.deletedAt.UTC()
		return guard, err
	})
	if err != nil {
		t.Fatalf("reading the keys of deleted mail: %v", err)
	}
	return held
}

// rowCountingTx is a transaction that counts each row its reads of many rows answer.
type rowCountingTx struct {
	pgx.Tx
	read *int
}

// Query reads the rows of sql, counting each one it answers.
func (c rowCountingTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	rows, err := c.Tx.Query(ctx, sql, args...)
	return countedRows{Rows: rows, read: c.read}, err
}

// countedRows is a read of many rows that counts each row it moves to.
type countedRows struct {
	pgx.Rows
	read *int
}

// Next moves to the next row and counts it.
func (c countedRows) Next() bool {
	next := c.Rows.Next()
	if next {
		*c.read++
	}
	return next
}

// rowsReadDeleting returns how many rows the reads of a delete of the mail id of thread answer, the delete rolled back.
func (it *ingestTest) rowsReadDeleting(thread, id uuid.UUID) int {
	it.t.Helper()
	read, tx := 0, beginTx(it.t, it.pool)
	_, err := deleteLockedThreadMail(it.t.Context(), rowCountingTx{Tx: tx, read: &read}, sdk.DefaultTenantID, it.box.id,
		threadDelete{thread: thread, message: &id, at: ingestNow, window: testSyncWindow})
	if err != nil {
		it.t.Fatalf("deleteLockedThreadMail() error = %v, want nil", err)
	}
	if err := tx.Rollback(it.t.Context()); err != nil {
		it.t.Fatalf("rolling the delete back: %v", err)
	}
	return read
}

// wantRejected fails the test unless err is the graph error of code and reason.
func wantRejected(t *testing.T, err error, code, reason string) {
	t.Helper()
	var rejected sdk.GraphError
	if !errors.As(err, &rejected) || rejected.Code != code || rejected.Reason != reason {
		t.Errorf("error = %v, want it rejected as %s for %s", err, code, reason)
	}
}

func TestDeleteMessagesRemovesTheMailWithItsThreadFilesLinksReferencesAndKeys(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	id := it.mailTitled("Invoice")
	row, files := storedRow(t, it.pool, id), it.attachmentsOf(id)

	deleted, err := it.deleteInTx(id)

	if err != nil || deleted != 1 {
		t.Fatalf("deleteMessages() = %d, %v, want 1 mail deleted", deleted, err)
	}
	if rows := countRows(t, it.pool, mailRowsOf, sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the workspace keeps %d rows of the mail, its thread and their parts, want none", rows)
	}
	for file, size := range map[uuid.UUID]int64{row.rawFileID: row.rawSize, files[0].fileID: files[0].size} {
		if state, queued := queueRow(t.Context(), it.pool, sdk.DefaultTenantID, file); state != "deleting" ||
			queued != size {
			t.Errorf("the file %v is queued as %q of %d bytes, want deleting with its %d", file, state, queued, size)
		}
	}
	if used := usageOf(t, it.pool, sdk.DefaultTenantID); used != 0 {
		t.Errorf("the workspace counts %d used bytes, want its text freed and its files left to the queue", used)
	}
}

func TestDeleteMessagesQueuesNoFileTheMailDidNotKeep(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.files.workspaceMaxBytes = 1 })
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	id := it.mailTitled("Invoice")
	if row := storedRow(t, it.pool, id); row.rawState != fileOverLimit {
		t.Fatalf("the original is %q, want %q in a full workspace", row.rawState, fileOverLimit)
	}

	deleted, err := it.deleteInTx(id)

	if err != nil || deleted != 1 {
		t.Fatalf("deleteMessages() = %d, %v, want 1 mail deleted", deleted, err)
	}
	if queued := queueSize(t, it.pool, sdk.DefaultTenantID); queued != 0 {
		t.Errorf("the queue holds %d files, want none for a mail that kept no file", queued)
	}
}

func TestDeleteMessagesGuardsTheKeyOfEachMailWithItsIDAndTheTimeItIsGiven(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	quote, invoice := it.mailTitled("Your quote"), it.mailTitled("Invoice")
	want := []guardRow{
		{mailboxID: it.box.id, key: storedRow(t, it.pool, quote).dedupeKey, messageID: quote, deletedAt: ingestNow},
		{mailboxID: it.box.id, key: storedRow(t, it.pool, invoice).dedupeKey, messageID: invoice, deletedAt: ingestNow},
	}
	slices.SortFunc(want, func(a, b guardRow) int { return bytes.Compare(a.key, b.key) })

	deleted, err := it.deleteInTx(quote, invoice)

	if err != nil || deleted != 2 {
		t.Fatalf("deleteMessages() = %d, %v, want 2 mails deleted", deleted, err)
	}
	if got := guardRows(t, it.pool, sdk.DefaultTenantID); !reflect.DeepEqual(got, want) {
		t.Errorf("the keys of deleted mail are %+v, want %+v", got, want)
	}
}

func TestDeleteMessagesGuardsNoKeyForAMailThatHoldsNone(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeEmptyThread(t, tx, sdk.DefaultTenantID, box)
	id := mailIn(t, tx, box, thread, 0)

	deleted, err := deleteMessages(t.Context(), tx, sdk.DefaultTenantID, []uuid.UUID{id}, ingestNow)

	if err != nil || deleted != 1 {
		t.Fatalf("deleteMessages() = %d, %v, want 1 mail deleted", deleted, err)
	}
	if guards := guardRows(t, tx, sdk.DefaultTenantID); len(guards) != 0 {
		t.Errorf("the keys of deleted mail are %+v, want none for a mail with no key", guards)
	}
}

func TestDeleteMessagesFailsWholeOnAKeyGuardedAlready(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	id := it.mailTitled("Your quote")
	if _, err := it.pool.Exec(t.Context(), `INSERT INTO plugin_email.deleted_dedupe_keys
		(tenant_id, mailbox_id, dedupe_key, deleted_message_id, deleted_at) VALUES ($1, $2, $3, $4, $5)`,
		sdk.DefaultTenantID, it.box.id, storedRow(t, it.pool, id).dedupeKey, uuid.Must(uuid.NewV7()), ingestNow,
	); err != nil {
		t.Fatalf("guarding the key by hand: %v", err)
	}

	_, err := it.deleteInTx(id)

	if code := pgCode(err); code != uniqueViolation {
		t.Errorf("deleteMessages() error = %v, want the guarded key to fail the delete", err)
	}
	if mails := it.mails(); mails != 1 {
		t.Errorf("the workspace holds %d mails, want the mail kept", mails)
	}
}

func TestDeleteMessagesFitsAThreadThatKeepsMailToTheMailLeft(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	thread, mails := it.threadOfThree("file:reply")

	deleted, err := it.deleteInTx(mails[0], mails[2])

	if err != nil || deleted != 2 {
		t.Fatalf("deleteMessages() = %d, %v, want 2 mails deleted", deleted, err)
	}
	subject, key, span := storedSpan(t, beginTx(t, it.pool), thread)
	left := mailReceived.Add(time.Hour)
	if subject != "Re: Your quote" || !bytes.Equal(key, subjectKeyOf("Re: Your quote")) ||
		!span.firstMessageAt.Equal(left) || !span.lastMessageAt.Equal(left) {
		t.Errorf("the thread left = %q, %x, %+v, want the subject, key and time of the reply left at %v",
			subject, key, span, left)
	}
}

func TestDeleteMessagesKeepsTheMessageIDOfADeletedMailAsAPlaceholderOfItsThread(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	thread, mails := it.threadOfThree("file:reply")

	if _, err := it.deleteInTx(mails[0]); err != nil {
		t.Fatalf("deleteMessages() error = %v, want nil", err)
	}

	held := keyRow(t, beginTx(t, it.pool), it.box, hashedKey(internetMessageIDKind, "quote@example.com"))
	if held.thread != thread || held.message != uuid.Nil {
		t.Errorf("the key of the deleted quote points at %+v, want the thread %v and no mail", held, thread)
	}
	it.ingestAt(quoteReplyMail("quote-third-reply@example.com", "Friday suits me."), "file:third-reply", 3*time.Hour)
	if joined := it.threadOf(it.mailByMessageID("quote-third-reply@example.com")); joined != thread {
		t.Errorf("a later reply to the deleted quote joined %v, want the thread %v it stays in", joined, thread)
	}
}

func TestDeleteMessagesTakesTheStoredBytesOffTheWorkspaceNeverBelowZero(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	quote, invoice := it.mailTitled("Your quote"), it.mailTitled("Invoice")
	invoiceBytes := storedRow(t, it.pool, invoice).storedBytes

	if _, err := it.deleteInTx(quote); err != nil {
		t.Fatalf("deleteMessages() error = %v, want nil", err)
	}
	if left := usageOf(t, it.pool, sdk.DefaultTenantID); left != invoiceBytes {
		t.Errorf("the workspace counts %d used bytes, want the invoice's %d left", left, invoiceBytes)
	}
	it.exec("UPDATE plugin_email.workspace_usage SET used_bytes = 5")
	if _, err := it.deleteInTx(invoice); err != nil {
		t.Fatalf("deleteMessages() error = %v, want nil", err)
	}

	if left := usageOf(t, it.pool, sdk.DefaultTenantID); left != 0 {
		t.Errorf("the workspace counts %d used bytes after a delete larger than its count, want 0", left)
	}
}

func TestDeleteThreadMailDeletesAFedMailAtOnceAndAnswersItsID(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	id := it.mailTitled("Your quote")

	deletion, err := it.deleteWhole(it.threadOf(id), &id, ingestNow)

	if err != nil || !slices.Equal(deletion.messages, []uuid.UUID{id}) || !deletion.threadDeleted {
		t.Errorf("deleteThreadMail() = %+v, %v, want %v deleted with its thread", deletion, err, id)
	}
	if rows := countRows(t, it.pool, mailRowsOf, sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the workspace keeps %d rows of the mail and its thread, want none", rows)
	}
}

func TestDeleteThreadMailDeletesAWholeThreadOfFedMailOldestFirst(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	thread, mails := it.threadOfThree("file:reply")

	deletion, err := it.deleteWhole(thread, nil, ingestNow)

	if err != nil || !slices.Equal(deletion.messages, mails) || !deletion.threadDeleted {
		t.Errorf("deleteThreadMail() = %+v, %v, want %v deleted with their thread", deletion, err, mails)
	}
	if left := it.mails(); left != 0 {
		t.Errorf("the workspace holds %d mails, want none", left)
	}
}

func TestDeleteThreadMailDeletesOneMailAndKeepsItsThread(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	thread, mails := it.threadOfThree("file:reply")

	deletion, err := it.deleteWhole(thread, &mails[1], ingestNow)

	if err != nil || !slices.Equal(deletion.messages, mails[1:2]) || deletion.threadDeleted {
		t.Errorf("deleteThreadMail() = %+v, %v, want %v deleted and the thread kept", deletion, err, mails[1])
	}
	kept, found, err := readThread(t.Context(), it.pool, sdk.DefaultTenantID, thread)
	if err != nil || !found || kept.messageCount != 2 || kept.latest.id != mails[2] {
		t.Errorf("readThread() = %+v, %t, %v, want the thread with its 2 mails left", kept, found, err)
	}
}

func TestADeleteOfOneMailReadsAsManyRowsInALongThreadAsInAShortOne(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	thread, mails := it.threadOfThree("file:reply")
	short := it.rowsReadDeleting(thread, mails[1])
	it.ingestAt(quoteReplyMail("quote-third-reply@example.com", "Friday suits me."), "file:third-reply", 3*time.Hour)
	it.ingestAt(quoteReplyMail("quote-fourth-reply@example.com", "The morning suits me."), "file:fourth-reply",
		4*time.Hour)
	for _, id := range []string{"quote-third-reply@example.com", "quote-fourth-reply@example.com"} {
		if joined := it.threadOf(it.mailByMessageID(id)); joined != thread {
			t.Fatalf("the reply %s sits in the thread %v, want the quote's %v", id, joined, thread)
		}
	}

	long := it.rowsReadDeleting(thread, mails[1])

	if long != short {
		t.Errorf("a delete of one mail read %d rows in a thread of five and %d in a thread of three, want as many",
			long, short)
	}
}

func TestDeleteThreadMailDeletesTheSeededMailAtOnce(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	seeded := it.seedWorkspace()
	thread := it.threadOf(seeded.mail(t, it, "seed:lamp-quote"))

	deletion, err := deleteThreadMail(t.Context(), it.pool, seeded.tenant,
		threadDelete{thread: thread, at: ingestNow, window: testSyncWindow})

	if err != nil || len(deletion.messages) != 3 || !deletion.threadDeleted {
		t.Errorf("deleteThreadMail() = %+v, %v, want the 3 seeded mails of the thread deleted", deletion, err)
	}
	if got, want := seeded.counts(t, it), [5]int{1, 4, 4, 1, 5}; got != want {
		t.Errorf("the seeded workspace holds %v, want %v", got, want)
	}
}

func TestDeleteThreadMailRejectsASyncedMailReceivedWithinTheWindow(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), syncedRef))
	id := it.mailTitled("Your quote")
	thread, windowAgo := it.threadOf(id), mailReceived.Add(testSyncWindow)

	_, mailErr := it.deleteWhole(thread, &id, windowAgo)
	_, threadErr := it.deleteWhole(thread, nil, windowAgo)

	for _, err := range []error{mailErr, threadErr} {
		var rejected sdk.GraphError
		if !errors.As(err, &rejected) || rejected.Code != "CONFLICT" || rejected.Reason != "inside_sync_window" ||
			rejected.Meta["window"] != "720h0m0s" {
			t.Errorf("deleteThreadMail() error = %v, want it rejected inside the window of 720h0m0s", err)
		}
	}
	if mails, guards := it.mails(), guardRows(t, it.pool, sdk.DefaultTenantID); mails != 1 || len(guards) != 0 {
		t.Errorf("the workspace holds %d mails and %d keys of deleted mail, want the mail kept as it was",
			mails, len(guards))
	}
	deletion, err := it.deleteWhole(thread, &id, windowAgo.Add(time.Second))
	if err != nil || !slices.Equal(deletion.messages, []uuid.UUID{id}) {
		t.Errorf("deleteThreadMail() a second past the window = %+v, %v, want %v deleted", deletion, err, id)
	}
}

func TestDeleteThreadMailRejectsAWholeThreadThatHoldsOneYoungSyncedMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	thread, mails := it.threadOfThree(syncedRef)
	at := mailReceived.Add(testSyncWindow).Add(time.Second)

	_, err := it.deleteWhole(thread, nil, at)

	wantRejected(t, err, "CONFLICT", "inside_sync_window")
	if left := it.mails(); left != 3 {
		t.Errorf("the workspace holds %d mails, want the whole thread kept", left)
	}
	deletion, err := it.deleteWhole(thread, &mails[2], at)
	if err != nil || !slices.Equal(deletion.messages, mails[2:]) || deletion.threadDeleted {
		t.Errorf("deleteThreadMail() of the fed reply = %+v, %v, want it deleted and the thread kept", deletion, err)
	}
}

func TestDeleteThreadMailRejectsAYoungMailThatHoldsAFedReferenceBesideASyncedOne(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	if outcome := it.ingest(it.incoming(lampMail(), syncedRef)); outcome != outcomeDuplicate {
		t.Fatalf("ingest() of the synced copy = %q, want %q", outcome, outcomeDuplicate)
	}
	id := it.mailTitled("Your quote")

	_, err := it.deleteWhole(it.threadOf(id), &id, ingestNow)

	wantRejected(t, err, "CONFLICT", "inside_sync_window")
	if left := it.mails(); left != 1 {
		t.Errorf("the workspace holds %d mails, want the mail kept", left)
	}
}

func TestDeleteThreadMailAnswersNotFoundForAThreadOrAMailTheWorkspaceDoesNotHold(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	quote, invoice := it.mailTitled("Your quote"), it.mailTitled("Invoice")
	unknown, noMailbox := uuid.Must(uuid.NewV7()), storeWorkspace(t, it.pool)

	for name, c := range map[string]struct {
		tenant  uuid.UUID
		thread  uuid.UUID
		message *uuid.UUID
		reason  string
	}{
		"an unknown thread":              {tenant: sdk.DefaultTenantID, thread: unknown, reason: "thread_not_found"},
		"a mail of an unknown thread":    {sdk.DefaultTenantID, unknown, &quote, "thread_not_found"},
		"a mail of another thread":       {sdk.DefaultTenantID, it.threadOf(quote), &invoice, "message_not_found"},
		"a workspace that holds no mail": {tenant: noMailbox, thread: it.threadOf(quote), reason: "thread_not_found"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := deleteThreadMail(t.Context(), it.pool, c.tenant,
				threadDelete{thread: c.thread, message: c.message, at: ingestNow, window: testSyncWindow})

			wantRejected(t, err, "NOT_FOUND", c.reason)
		})
	}
	if left := it.mails(); left != 2 {
		t.Errorf("the workspace holds %d mails, want both kept", left)
	}
}

func TestCheckSyncWindowRejectsASyncedMailFromTheStartOfTheWindowOn(t *testing.T) {
	t.Parallel()

	at := ingestNow
	start := at.Add(-testSyncWindow)

	for name, c := range map[string]struct {
		mail     mailToDelete
		rejected bool
	}{
		"a synced mail received as the window starts":   {mailToDelete{receivedAt: start, synced: true}, true},
		"a synced mail received inside the window":      {mailToDelete{receivedAt: at, synced: true}, true},
		"a synced mail received just before the window": {mailToDelete{receivedAt: start.Add(-1), synced: true}, false},
		"a fed mail received inside the window":         {mailToDelete{receivedAt: at}, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := checkSyncWindow([]mailToDelete{{receivedAt: start.Add(-time.Hour)}, c.mail}, at, testSyncWindow)

			if c.rejected {
				wantRejected(t, err, "CONFLICT", "inside_sync_window")
			}
			if !c.rejected && err != nil {
				t.Errorf("checkSyncWindow() error = %v, want nil", err)
			}
		})
	}
}

func TestDeleteFilesTurnsAnAttachmentDeletedAndKeepsWhatTheMailCarried(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	id := it.mailTitled("Invoice")
	before, row, used := it.attachmentsOf(id)[0], storedRow(t, it.pool, id), usageOf(t, it.pool, sdk.DefaultTenantID)
	file := it.attachmentIDs(id)[0]

	deletion, err := it.freeFiles(filesDelete{message: id, attachment: &file})

	if err != nil || deletion != (filesDeletion{attachment: true}) {
		t.Fatalf("deleteFiles() = %+v, %v, want the attachment deleted", deletion, err)
	}
	want := before
	want.fileID, want.state = uuid.Nil, fileDeleted
	if got := it.attachmentsOf(id)[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("the attachment = %+v, want %+v with its name, type, size and hash kept", got, want)
	}
	if state, size := queueRow(t.Context(), it.pool, sdk.DefaultTenantID, before.fileID); state != "deleting" ||
		size != before.size {
		t.Errorf("the file is queued as %q of %d bytes, want deleting with its %d", state, size, before.size)
	}
	after := storedRow(t, it.pool, id)
	if after.storedBytes != row.storedBytes-before.size || after.rawState != fileStored {
		t.Errorf("the mail keeps %d bytes and its original as %q, want %d and the original kept",
			after.storedBytes, after.rawState, row.storedBytes-before.size)
	}
	if left := usageOf(t, it.pool, sdk.DefaultTenantID); left != used-before.size {
		t.Errorf("the workspace counts %d used bytes, want %d", left, used-before.size)
	}
}

func TestDeleteFilesChangesNothingForAFileDeletedBeforeOrNeverKept(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.files.attachmentMaxBytes = 16 })
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march", strings.Repeat("x", 40)), "file:invoice"))
	id := it.mailTitled("Invoice")
	files := it.attachmentIDs(id)
	if _, err := it.freeFiles(filesDelete{message: id, attachment: &files[0]}); err != nil {
		t.Fatalf("deleteFiles() error = %v, want nil", err)
	}
	row, used := storedRow(t, it.pool, id), usageOf(t, it.pool, sdk.DefaultTenantID)

	for _, file := range files {
		deletion, err := it.freeFiles(filesDelete{message: id, attachment: &file})

		if err != nil || deletion != (filesDeletion{}) {
			t.Errorf("deleteFiles() of %v = %+v, %v, want nothing deleted", file, deletion, err)
		}
	}
	if after := storedRow(t, it.pool, id); after.storedBytes != row.storedBytes ||
		usageOf(t, it.pool, sdk.DefaultTenantID) != used {
		t.Errorf("the mail keeps %d bytes, want %d and the workspace's count kept", after.storedBytes, row.storedBytes)
	}
	states := []string{it.attachmentsOf(id)[0].state, it.attachmentsOf(id)[1].state}
	if !slices.Equal(states, []string{fileDeleted, fileTooLarge}) {
		t.Errorf("the files are %q, want the deleted one and the one never kept as they were", states)
	}
}

func TestDeleteFilesTurnsTheOriginalDeletedAndKeepsItsHashAndSize(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	id := it.mailTitled("Invoice")
	row, files, used := storedRow(t, it.pool, id), it.attachmentsOf(id), usageOf(t, it.pool, sdk.DefaultTenantID)

	deletion, err := it.freeFiles(filesDelete{message: id, original: true})

	if err != nil || deletion != (filesDeletion{original: true}) {
		t.Fatalf("deleteFiles() = %+v, %v, want the original deleted", deletion, err)
	}
	want := row
	want.rawFileID, want.rawState, want.storedBytes = uuid.Nil, fileDeleted, row.storedBytes-row.rawSize
	if got := storedRow(t, it.pool, id); !reflect.DeepEqual(got, want) {
		t.Errorf("the mail = %+v, want %+v with the hash and size of its original kept", got, want)
	}
	if state, size := queueRow(t.Context(), it.pool, sdk.DefaultTenantID, row.rawFileID); state != "deleting" ||
		size != row.rawSize {
		t.Errorf("the original is queued as %q of %d bytes, want deleting with its %d", state, size, row.rawSize)
	}
	if got := it.attachmentsOf(id); !reflect.DeepEqual(got, files) {
		t.Errorf("the attachments = %+v, want them kept as %+v", got, files)
	}
	if left := usageOf(t, it.pool, sdk.DefaultTenantID); left != used-row.rawSize {
		t.Errorf("the workspace counts %d used bytes, want %d", left, used-row.rawSize)
	}
}

func TestDeleteFilesDeletesAnAttachmentAndTheOriginalInOneCall(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	id := it.mailTitled("Invoice")
	file := it.attachmentIDs(id)[0]

	deletion, err := it.freeFiles(filesDelete{message: id, attachment: &file, original: true})

	if err != nil || deletion != (filesDeletion{attachment: true, original: true}) {
		t.Fatalf("deleteFiles() = %+v, %v, want the attachment and the original deleted", deletion, err)
	}
	row := storedRow(t, it.pool, id)
	if text := textBytes(t, row); row.storedBytes != text || usageOf(t, it.pool, sdk.DefaultTenantID) != text {
		t.Errorf("the mail keeps %d bytes, want its %d bytes of text alone in the mail and the workspace",
			row.storedBytes, text)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting", "deleting"}) {
		t.Errorf("the queue holds %q, want both files waiting for deletion", states)
	}
}

func TestDeleteFilesRejectsADeleteThatNamesNoFile(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	id := it.mailTitled("Invoice")
	row := storedRow(t, it.pool, id)

	_, err := it.freeFiles(filesDelete{message: id})

	wantRejected(t, err, "VALIDATION", "nothing_to_delete")
	if got := storedRow(t, it.pool, id); !reflect.DeepEqual(got, row) {
		t.Errorf("the mail = %+v, want it kept as %+v", got, row)
	}
}

func TestDeleteFilesAnswersNotFoundForAMailOrAFileTheWorkspaceDoesNotHold(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	it.ingest(it.incoming(lampMail(), "file:quote"))
	invoice, quote := it.mailTitled("Invoice"), it.mailTitled("Your quote")
	file, unknown, noMailbox := it.attachmentIDs(invoice)[0], uuid.Must(uuid.NewV7()), storeWorkspace(t, it.pool)
	home := sdk.DefaultTenantID

	for name, c := range map[string]struct {
		tenant  uuid.UUID
		request filesDelete
		reason  string
	}{
		"the original of an unknown mail":    {home, filesDelete{message: unknown, original: true}, "message_not_found"},
		"a file named under an unknown mail": {home, filesDelete{message: unknown, attachment: &file}, "message_not_found"},
		"a file of another mail":             {home, filesDelete{message: quote, attachment: &file}, "attachment_not_found"},
		"a workspace that holds no mail":     {noMailbox, filesDelete{message: invoice, original: true}, "message_not_found"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := deleteFiles(t.Context(), it.pool, c.tenant, c.request)

			wantRejected(t, err, "NOT_FOUND", c.reason)
		})
	}
	row, kept := storedRow(t, it.pool, invoice), it.attachmentsOf(invoice)[0]
	if row.rawState != fileStored || kept.state != fileStored {
		t.Errorf("the invoice keeps its original as %q and its file as %q, want both kept", row.rawState, kept.state)
	}
}

func TestDeletingAnAttachmentThenItsMailFreesEachByteOnce(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	id := it.mailTitled("Invoice")
	row, kept, file := storedRow(t, it.pool, id), it.attachmentsOf(id)[0], it.attachmentIDs(id)[0]
	if _, err := it.freeFiles(filesDelete{message: id, attachment: &file}); err != nil {
		t.Fatalf("deleteFiles() error = %v, want nil", err)
	}

	if _, err := it.deleteWhole(row.threadID, &id, ingestNow); err != nil {
		t.Fatalf("deleteThreadMail() error = %v, want nil", err)
	}

	if queued := queueSize(t, it.pool, sdk.DefaultTenantID); queued != 2 {
		t.Errorf("the queue holds %d files, want the attachment and the original once each", queued)
	}
	for queuedFile, size := range map[uuid.UUID]int64{kept.fileID: kept.size, row.rawFileID: row.rawSize} {
		if state, held := queueRow(t.Context(), it.pool, sdk.DefaultTenantID, queuedFile); state != "deleting" ||
			held != size {
			t.Errorf("the file %v is queued as %q of %d bytes, want deleting with its %d", queuedFile, state, held, size)
		}
	}
	if used := usageOf(t, it.pool, sdk.DefaultTenantID); used != 0 {
		t.Errorf("the workspace counts %d used bytes, want 0", used)
	}
}

func TestTheSweepDeletesEveryFileADeleteFreedAndKeepsTheRest(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	it.ingest(it.incoming(lampMail(), "file:quote"))
	invoiceID := it.mailTitled("Invoice")
	invoice, quote := storedRow(t, it.pool, invoiceID), storedRow(t, it.pool, it.mailTitled("Your quote"))
	attachment, file := it.attachmentsOf(invoiceID)[0].fileID, it.attachmentIDs(invoiceID)[0]
	if _, err := it.freeFiles(filesDelete{message: invoiceID, attachment: &file}); err != nil {
		t.Fatalf("deleteFiles() error = %v, want nil", err)
	}
	if _, err := it.deleteWhole(quote.threadID, nil, ingestNow); err != nil {
		t.Fatalf("deleteThreadMail() error = %v, want nil", err)
	}

	it.sweep()

	for _, gone := range []uuid.UUID{attachment, quote.rawFileID} {
		if !fileGone(t, it.store, sdk.DefaultTenantID, gone) {
			t.Errorf("the store keeps the freed file %v, want it deleted", gone)
		}
	}
	if kept := storedBytes(t, it.store, sdk.DefaultTenantID, invoice.rawFileID); len(kept) != int(invoice.rawSize) {
		t.Errorf("the store keeps %d bytes of the invoice's original, want its %d", len(kept), invoice.rawSize)
	}
	if queued := queueSize(t, it.pool, sdk.DefaultTenantID); queued != 0 {
		t.Errorf("the queue holds %d files after the sweep, want none", queued)
	}
}

func TestTwoDeletesOfOneMailboxWaitOnItsLock(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	quote, invoice := it.mailTitled("Your quote"), it.mailTitled("Invoice")
	thread := it.threadOf(quote)
	holder := it.holdMailbox()
	whole, files := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := it.deleteWhole(thread, &quote, ingestNow)
		whole <- err
	}()
	go func() {
		_, err := it.freeFiles(filesDelete{message: invoice, original: true})
		files <- err
	}()
	it.awaitLockWaits(2)
	if mails := it.mails(); mails != 2 || storedRow(t, it.pool, invoice).rawState != fileStored {
		t.Errorf("the workspace holds %d mails while the lock is held, want both kept as they were", mails)
	}

	if err := holder.Rollback(t.Context()); err != nil {
		t.Fatalf("releasing the mailbox: %v", err)
	}

	if err := errors.Join(<-whole, <-files); err != nil {
		t.Fatalf("the deletes answered %v, want nil", err)
	}
	if mails := it.mails(); mails != 1 || storedRow(t, it.pool, invoice).rawState != fileDeleted {
		t.Errorf("the workspace holds %d mails, want the quote deleted and the invoice's original deleted", mails)
	}
}

func TestWasDeletedAnswersWhetherTheMailboxDeletedAMailOfTheKeyOrTheFailureOfItsRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	id := it.mailTitled("Your quote")
	key := storedRow(t, it.pool, id).dedupeKey
	before, beforeErr := wasDeleted(t.Context(), it.pool, sdk.DefaultTenantID, it.box.id, key)
	if _, err := it.deleteInTx(id); err != nil {
		t.Fatalf("deleteMessages() error = %v, want nil", err)
	}

	after, afterErr := wasDeleted(t.Context(), it.pool, sdk.DefaultTenantID, it.box.id, key)
	other, otherErr := wasDeleted(t.Context(), it.pool, sdk.DefaultTenantID, it.box.id, sum("another mail"))
	_, endedErr := wasDeleted(t.Context(), endedTx(t), sdk.DefaultTenantID, it.box.id, key)

	if before || beforeErr != nil || !after || afterErr != nil || other || otherErr != nil {
		t.Errorf("wasDeleted() before, after and of another key = %t %v, %t %v, %t %v, want false, true, false",
			before, beforeErr, after, afterErr, other, otherErr)
	}
	if !errors.Is(endedErr, pgx.ErrTxClosed) {
		t.Errorf("wasDeleted() on an ended transaction error = %v, want %v", endedErr, pgx.ErrTxClosed)
	}
}

func TestEachStepOfAWholeDeleteAnswersTheFailureOfItsStatement(t *testing.T) {
	t.Parallel()

	for name, fragment := range map[string]string{
		"guardDeletedKeys": "INSERT INTO plugin_email.deleted_dedupe_keys",
		"queueMailFiles":   "m.raw_file_id AS file_id",
		"deleteMailRows":   "DELETE FROM plugin_email.attachments a",
		"fixThreadsLeft":   "DELETE FROM plugin_email.threads t",
		"fixThreadLeft":    "UPDATE plugin_email.threads SET subject",
		"takeStoredBytes":  "SET used_bytes = GREATEST(used_bytes - $2, 0)",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			_, mails := it.threadOfThree("file:reply")
			tx := beginTx(t, it.pool)

			_, err := deleteMessages(t.Context(), failingTx{Tx: tx, fragment: fragment}, sdk.DefaultTenantID,
				mails[:1], ingestNow)

			if code := pgCode(err); code != undefinedFunction {
				t.Errorf("deleteMessages() with %s failing error = %v, want the failure of its statement", name, err)
			}
		})
	}
}

func TestEachStepOfADeleteOfThreadMailAnswersTheFailureOfItsStatement(t *testing.T) {
	t.Parallel()

	for name, fragment := range map[string]string{
		"lockMailbox":      "FOR NO KEY UPDATE",
		"readMailToDelete": "starts_with(r.provider_ref",
		"deleteMessages":   "INSERT INTO plugin_email.deleted_dedupe_keys",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			it.ingest(it.incoming(lampMail(), "file:quote"))
			thread := it.threadOf(it.mailTitled("Your quote"))
			tx := beginTx(t, it.pool)

			_, err := deleteLockedThreadMail(t.Context(), failingTx{Tx: tx, fragment: fragment}, sdk.DefaultTenantID,
				it.box.id, threadDelete{thread: thread, at: ingestNow, window: testSyncWindow})

			if code := pgCode(err); code != undefinedFunction {
				t.Errorf("deleteLockedThreadMail() with %s failing error = %v, want the failure of its statement",
					name, err)
			}
		})
	}
}

func TestADeleteOfOneMailAnswersTheFailureOfItsRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	id := it.mailTitled("Your quote")
	tx := beginTx(t, it.pool)

	_, err := deleteLockedThreadMail(t.Context(), failingTx{Tx: tx, fragment: "AND o.id <> $3"}, sdk.DefaultTenantID,
		it.box.id, threadDelete{thread: it.threadOf(id), message: &id, at: ingestNow, window: testSyncWindow})

	if code := pgCode(err); code != undefinedFunction {
		t.Errorf("deleteLockedThreadMail() of one mail with its read failing error = %v, want the failure of the read",
			err)
	}
}

func TestEachStepOfADeleteOfFilesAnswersTheFailureOfItsStatement(t *testing.T) {
	t.Parallel()

	for name, fragment := range map[string]string{
		"lockMailbox":          "FOR NO KEY UPDATE",
		"lockMailToFree":       "AND id = $2 FOR UPDATE",
		"deleteAttachmentFile": "UPDATE plugin_email.attachments a SET state",
		"deleteOriginalFile":   "SET raw_state = 'deleted'",
		"takeFreedBytes":       "SET stored_bytes = GREATEST",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
			id := it.mailTitled("Invoice")
			file := it.attachmentIDs(id)[0]
			tx := beginTx(t, it.pool)

			_, err := deleteLockedFiles(t.Context(), failingTx{Tx: tx, fragment: fragment}, sdk.DefaultTenantID,
				it.box.id, filesDelete{message: id, attachment: &file, original: true})

			if code := pgCode(err); code != undefinedFunction {
				t.Errorf("deleteLockedFiles() with %s failing error = %v, want the failure of its statement",
					name, err)
			}
		})
	}
}

func TestInMailboxTxHandsTheMailboxOfTheWorkspaceToItsStepOrAnswersMissing(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	noMailbox := storeWorkspace(t, it.pool)
	var handed []uuid.UUID
	step := func(tx pgx.Tx, mailboxID uuid.UUID) (uuid.UUID, error) {
		handed = append(handed, mailboxID)
		return mailboxID, lockMailbox(t.Context(), tx, sdk.DefaultTenantID, mailboxID)
	}

	held, heldErr := inMailboxTx(t.Context(), it.pool, sdk.DefaultTenantID, threadNotFound, step)
	_, missingErr := inMailboxTx(t.Context(), it.pool, noMailbox, messageNotFound, step)

	if held != it.box.id || heldErr != nil || !slices.Equal(handed, []uuid.UUID{it.box.id}) {
		t.Errorf("inMailboxTx() = %v, %v after its step was handed %v, want the mailbox %v handed once",
			held, heldErr, handed, it.box.id)
	}
	wantRejected(t, missingErr, "NOT_FOUND", "message_not_found")
}

func TestTheDeletesAnswerTheFailureOfTheirMailboxRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.pool.Close()

	_, wholeErr := it.deleteWhole(uuid.Must(uuid.NewV7()), nil, ingestNow)
	_, filesErr := it.freeFiles(filesDelete{message: uuid.Must(uuid.NewV7()), original: true})

	for name, err := range map[string]error{"deleteThreadMail": wholeErr, "deleteFiles": filesErr} {
		var rejected sdk.GraphError
		if err == nil || errors.As(err, &rejected) {
			t.Errorf("%s() over a closed pool error = %v, want the failure of the mailbox read", name, err)
		}
	}
}

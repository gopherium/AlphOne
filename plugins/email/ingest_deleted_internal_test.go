// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// guardRead is a fragment of the statement that reads whether a mailbox deleted a mail of a key.
const guardRead = "SELECT FROM plugin_email.deleted_dedupe_keys"

// queueDeleting is a fragment of the statement that queues the files of a run for deletion.
const queueDeleting = "SELECT $1, queued.file_id, 'deleting', queued.size"

// failuresAt counts the failures a mailbox records at one reference.
const failuresAt = "SELECT count(*) FROM plugin_email.failed_mail WHERE provider_ref = $1"

// heldState is what the default workspace of an ingest test holds: rows of mail, guarded keys, files, events and bytes.
type heldState struct {
	rows   int
	guards int
	saves  int
	events int
	used   int64
}

// deleteStored deletes the stored mail id of the default workspace at ingestNow, failing the test on an error.
func (it *ingestTest) deleteStored(id uuid.UUID) {
	it.t.Helper()
	if _, err := it.deleteWhole(it.threadOf(id), &id, ingestNow); err != nil {
		it.t.Fatalf("deleteThreadMail() error = %v, want nil", err)
	}
}

// seen returns what the default workspace of the test holds now.
func (it *ingestTest) seen() heldState {
	it.t.Helper()
	return heldState{
		rows: it.rows(sdk.DefaultTenantID), guards: len(guardRows(it.t, it.pool, sdk.DefaultTenantID)),
		saves: len(it.store.savedTypes()), events: len(it.events()), used: usageOf(it.t, it.pool, sdk.DefaultTenantID),
	}
}

// guardByHand keeps key as the key of a mail the test's mailbox deleted.
func (it *ingestTest) guardByHand(key []byte) {
	it.t.Helper()
	if _, err := it.pool.Exec(it.t.Context(), `INSERT INTO plugin_email.deleted_dedupe_keys
		(tenant_id, mailbox_id, dedupe_key, deleted_message_id, deleted_at) VALUES ($1, $2, $3, $4, $5)`,
		sdk.DefaultTenantID, it.box.id, key, uuid.Must(uuid.NewV7()), ingestNow); err != nil {
		it.t.Fatalf("guarding the key by hand: %v", err)
	}
}

func TestIngestAnswersAlreadyDeletedForAMailItsMailboxDeletedAndKeepsNothing(t *testing.T) {
	t.Parallel()

	for name, ref := range map[string]string{
		"the reference the deleted mail held": "file:invoice", "a reference of another folder": "file:archive",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
			it.deleteStored(it.mailTitled("Invoice"))
			before := it.seen()

			outcome := it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), ref))

			if outcome != outcomeAlreadyDeleted {
				t.Errorf("ingest() = %q, want %q", outcome, outcomeAlreadyDeleted)
			}
			if after := it.seen(); after != before {
				t.Errorf("ingest() left %+v, want %+v as the delete left it, with no row and no file", after, before)
			}
		})
	}
}

func TestPreviewAnswersAlreadyDeletedForAMailItsMailboxDeleted(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	it.deleteStored(it.mailTitled("Your quote"))
	before := it.seen()

	outcome, err := it.ingester.preview(t.Context(), sdk.DefaultTenantID, it.box, it.incoming(lampMail(), "file:quote"))

	if err != nil || outcome != outcomeAlreadyDeleted {
		t.Errorf("preview() = %q, %v, want %q", outcome, err, outcomeAlreadyDeleted)
	}
	if after := it.seen(); after != before {
		t.Errorf("preview() left %+v, want %+v with nothing written", after, before)
	}
}

func TestIngestAnswersAlreadyDeletedForASentCopyOfADeletedMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(infoMail(receivedLine), "file:inbox"))
	it.deleteStored(it.mailTitled("Your quote"))
	sent := it.incoming(infoMail(), "file:sent")
	sent.SentCopy = true

	outcome := it.ingest(sent)

	if mails := it.mails(); outcome != outcomeAlreadyDeleted || mails != 0 {
		t.Errorf("ingest() of the Sent copy = %q with %d mails, want %q and no mail", outcome, mails,
			outcomeAlreadyDeleted)
	}
}

func TestADeletedMailNeverStopsTheSameMailInAnotherWorkspace(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	acme, box := it.addWorkspace()
	it.ingest(it.incoming(lampMail(), "file:quote"))
	it.deleteStored(it.mailTitled("Your quote"))

	inAcme := it.ingestIn(acme, box, lampMail(), "file:quote")
	atHome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if inAcme != outcomeStored || atHome != outcomeAlreadyDeleted {
		t.Errorf("the deleted mail fed to Acme and to its own workspace answered %q and %q, want %q and %q",
			inAcme, atHome, outcomeStored, outcomeAlreadyDeleted)
	}
}

func TestIngestAnswersAlreadyDeletedAndQueuesItsFilesForAMailDeletedWhileItWaitedToStoreIt(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	holder := it.holdMailbox()
	outcomes, errs := make(chan Outcome, 1), make(chan error, 1)
	go func() {
		outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID,
			it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
		outcomes <- outcome
		errs <- err
	}()
	it.awaitLockWaits(1)
	mail, row, kept := it.prepareMail(invoiceMail("%PDF-1.4 march"), "file:archive", false)
	stored, err := it.ingester.writeMail(t.Context(), holder, sdk.DefaultTenantID, mail, row, kept)
	if err != nil {
		t.Fatalf("writeMail() of the copy the delete takes error = %v, want nil", err)
	}
	if _, err := deleteMessages(t.Context(), holder, sdk.DefaultTenantID, []uuid.UUID{stored.id}, ingestNow); err != nil {
		t.Fatalf("deleteMessages() error = %v, want nil", err)
	}
	if err := holder.Commit(t.Context()); err != nil {
		t.Fatalf("committing the delete: %v", err)
	}

	outcome, err := <-outcomes, <-errs

	if err != nil || outcome != outcomeAlreadyDeleted {
		t.Errorf("ingest() = %q, %v, want %q", outcome, err, outcomeAlreadyDeleted)
	}
	if mails, events := it.mails(), it.events(); mails != 0 || len(events) != 0 {
		t.Errorf("the workspace holds %d mails and %d events, want none", mails, len(events))
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting", "deleting", "deleting", "deleting"}) {
		t.Errorf("the queue holds %q, want the files of the run and of the deleted mail queued for deletion", states)
	}
	if failures := countRows(t, it.pool, failuresAt, "file:invoice"); failures != 0 {
		t.Errorf("the run left %d failures, want none", failures)
	}
	it.sweep()
	if queued := queueSize(t, it.pool, sdk.DefaultTenantID); queued != 0 {
		t.Errorf("the queue holds %d files after the sweep, want every file of the run deleted", queued)
	}
}

func TestIngestAnswersAlreadyDeletedAndRecordsNoFailureForACopyDeletedWhileItWaited(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:inbox"))
	id := it.mailTitled("Your quote")
	holder := it.holdMailbox()
	outcomes, errs := make(chan Outcome, 1), make(chan error, 1)
	go func() {
		outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID,
			it.incoming(lampMail(receivedLine), "file:archive"))
		outcomes <- outcome
		errs <- err
	}()
	it.awaitLockWaits(1)
	if _, err := deleteMessages(t.Context(), holder, sdk.DefaultTenantID, []uuid.UUID{id}, ingestNow); err != nil {
		t.Fatalf("deleteMessages() error = %v, want nil", err)
	}
	if err := holder.Commit(t.Context()); err != nil {
		t.Fatalf("committing the delete: %v", err)
	}

	outcome, err := <-outcomes, <-errs

	if err != nil || outcome != outcomeAlreadyDeleted {
		t.Errorf("ingest() = %q, %v, want %q", outcome, err, outcomeAlreadyDeleted)
	}
	if failures := countRows(t, it.pool, failuresAt, "file:archive"); failures != 0 {
		t.Errorf("the copy left %d failures, want none for a mail deleted on purpose", failures)
	}
	if rows := countRows(t, it.pool, mailRowsOf, sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the workspace keeps %d rows of mail, want the copy to add none", rows)
	}
}

func TestIngestAnswersDuplicateWithNoFileForACopyOfAMailWhoseFilesWereDeleted(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	id := it.mailTitled("Invoice")
	file := it.attachmentIDs(id)[0]
	if _, err := it.freeFiles(filesDelete{message: id, attachment: &file, original: true}); err != nil {
		t.Fatalf("deleteFiles() error = %v, want nil", err)
	}
	before := it.seen()

	outcome := it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:archive"))

	if outcome != outcomeDuplicate {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeDuplicate)
	}
	if after := it.seen(); after.saves != before.saves || after.used != before.used {
		t.Errorf("the copy saved %d files and left %d used bytes, want no file and the %d bytes as they were",
			after.saves-before.saves, after.used, before.used)
	}
	row, files := storedRow(t, it.pool, id), it.attachmentsOf(id)
	if row.rawState != fileDeleted || files[0].state != fileDeleted {
		t.Errorf("the mail keeps its original as %q and its file as %q, want both deleted", row.rawState, files[0].state)
	}
}

func TestIngestAndPreviewReportADedupeReadThatFailsPastTheGuard(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.exec("DROP TABLE plugin_email.messages CASCADE")

	_, ingestErr := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))
	_, previewErr := it.ingester.preview(t.Context(), sdk.DefaultTenantID, it.box, it.incoming(lampMail(), "file:quote"))

	for name, err := range map[string]error{"ingest": ingestErr, "preview": previewErr} {
		if pgCode(err) != "42P01" || !strings.HasPrefix(err.Error(), "email: read the mail of a dedupe key: ") {
			t.Errorf("%s() error = %v, want the dedupe read past the guard to fail on the missing table", name, err)
		}
	}
}

func TestTheGuardReadsOfIngestAnswerTheFailureOfTheirStatement(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		code string
		run  func(it *ingestTest) error
	}{
		"the read before the dedupe read": {code: "42P01", run: func(it *ingestTest) error {
			it.exec("DROP TABLE plugin_email.deleted_dedupe_keys")
			_, err := it.ingester.ingest(it.t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))
			return err
		}},
		"the read before the dedupe read of a preview": {code: "42P01", run: func(it *ingestTest) error {
			it.exec("DROP TABLE plugin_email.deleted_dedupe_keys")
			_, err := it.ingester.preview(it.t.Context(), sdk.DefaultTenantID, it.box,
				it.incoming(lampMail(), "file:quote"))
			return err
		}},
		"the read under the lock of a new mail": {code: undefinedFunction, run: func(it *ingestTest) error {
			mail, row, kept := it.prepareMail(lampMail(), "file:quote", false)
			tx := failingTx{Tx: beginTx(it.t, it.pool), fragment: guardRead}
			_, err := it.ingester.writeMail(it.t.Context(), tx, sdk.DefaultTenantID, mail, row, kept)
			return err
		}},
		"the queue of a new mail deleted while it waited": {code: undefinedFunction, run: func(it *ingestTest) error {
			mail, row, kept := it.prepareMail(lampMail(), "file:quote", false)
			it.guardByHand(mail.dedupeKey)
			tx := failingTx{Tx: beginTx(it.t, it.pool), fragment: queueDeleting}
			_, err := it.ingester.writeMail(it.t.Context(), tx, sdk.DefaultTenantID, mail, row, kept)
			return err
		}},
		"the read under the lock of a copy": {code: undefinedFunction, run: func(it *ingestTest) error {
			it.ingest(it.incoming(lampMail(), "file:inbox"))
			mail, err := it.ingester.readIncoming(it.t.Context(), it.box, it.incoming(lampMail(receivedLine), "file:archive"))
			if err != nil {
				it.t.Fatalf("readIncoming() error = %v, want nil", err)
			}
			tx := failingTx{Tx: beginTx(it.t, it.pool), fragment: guardRead}
			_, err = it.ingester.writeCopy(it.t.Context(), tx, sdk.DefaultTenantID, mail)
			return err
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := c.run(newIngestTest(t))

			if code := pgCode(err); code != c.code {
				t.Errorf("%s error = %v, want the failure of its statement, SQLSTATE %s", name, err, c.code)
			}
		})
	}
}

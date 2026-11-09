// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

func TestIngestRollsBackAndQueuesEveryFileWhenTheSweepTookThem(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.store.hook = func(ctx context.Context, id uuid.UUID) error {
		_, err := it.pool.Exec(ctx, "UPDATE plugin_email.pending_files SET state = 'deleting' WHERE file_id = $1", id)
		return err
	}

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errFilesSwept) {
		t.Errorf("ingest() error = %v, want %v", err, errFilesSwept)
	}
	if mails := it.mails(); mails != 0 {
		t.Errorf("the workspace holds %d mails, want the commit rolled back", mails)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
	if failure := heldFailure(t, it.pool, it.box, "file:quote"); failure.reason != temporaryDatabaseError {
		t.Errorf("the failure is %+v, want a temporary one", failure)
	}
}

func TestIngestSetsAsideAMailTheDatabaseRefusesForGood(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.refuseNewMail("22023")

	outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if outcome != outcomeFailed {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeFailed)
	}
	failure := heldFailure(t, it.pool, it.box, "file:quote")
	if failure.reason != permanentDatabaseError || failure.attempts != 1 || !failure.firstFailedAt.Equal(ingestNow) {
		t.Errorf("the failure is %+v, want one permanent attempt at %v", failure, ingestNow)
	}
	var held string
	if err := it.pool.QueryRow(t.Context(), "SELECT to_jsonb(f)::text FROM plugin_email.failed_mail f").
		Scan(&held); err != nil || strings.Contains(held, "@") || strings.Contains(held, "refused") {
		t.Errorf("the failure row holds %s, %v, want no address and no error text", held, err)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
	if mails, events := it.mails(), it.events(); mails != 0 || len(events) != 0 {
		t.Errorf("the workspace holds %d mails and %d events, want none", mails, len(events))
	}
}

func TestIngestSetsAsideAMailOnAnOutageAndAnswersItsError(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.refuseNewMail("40001")

	outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if pgCode(err) != "40001" || outcome != "" {
		t.Errorf("ingest() = %q, %v, want no outcome and the error the caller retries", outcome, err)
	}
	if failure := heldFailure(t, it.pool, it.box, "file:quote"); failure.reason != temporaryDatabaseError {
		t.Errorf("the failure is %+v, want a temporary one", failure)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
}

func TestIngestSetsAsideAMailWhoseFileTheStoreCannotSave(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.store.hook = func(context.Context, uuid.UUID) error { return errStoreDown }

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errFileStore) || !errors.Is(err, errStoreDown) {
		t.Errorf("ingest() error = %v, want %v and %v", err, errFileStore, errStoreDown)
	}
	if failure := heldFailure(t, it.pool, it.box, "file:quote"); failure.reason != fileStoreError {
		t.Errorf("the failure is %+v, want a file store one", failure)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
}

func TestIngestAnswersBothErrorsWhenItCannotSetTheMailAside(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.refuseNewMail("22023")
	it.exec("ALTER TABLE plugin_email.failed_mail ADD CONSTRAINT refuse_failures CHECK (false) NOT VALID")

	outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if outcome != "" || pgCode(err) != "22023" || !strings.Contains(fmt.Sprint(err), "SQLSTATE 23514") {
		t.Errorf("ingest() = %q, %v, want no outcome and both the refused mail and the refused failure", outcome, err)
	}
}

func TestIngestSetsAsideAMailWhoseDeadlinePassesBeforeItsCommit(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	holder := beginTx(t, it.pool)
	if _, err := holder.Exec(t.Context(), "SELECT id FROM plugin_email.mailboxes WHERE id = $1 FOR NO KEY UPDATE",
		it.box.id); err != nil {
		t.Fatalf("locking the mailbox: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), callerDeadline)
	defer cancel()
	ran := make(chan error, 1)
	go func() {
		_, err := it.ingester.ingest(ctx, sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))
		ran <- err
	}()
	it.awaitLockWaits(1)

	err := <-ran

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ingest() error = %v, want the deadline of its caller", err)
	}
	failure := heldFailure(t, it.pool, it.box, "file:quote")
	if failure.reason != temporaryDatabaseError || failure.attempts != 1 || !failure.firstFailedAt.Equal(ingestNow) {
		t.Errorf("the failure is %+v, want one temporary attempt at %v", failure, ingestNow)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
}

func TestIngestTriesAgainAMailWhoseOnlyRunWasCancelled(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	holder := beginTx(t, it.pool)
	if _, err := holder.Exec(t.Context(), "SELECT id FROM plugin_email.mailboxes WHERE id = $1 FOR NO KEY UPDATE",
		it.box.id); err != nil {
		t.Fatalf("locking the mailbox: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	ran := make(chan error, 1)
	go func() {
		_, err := it.ingester.ingest(ctx, sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))
		ran <- err
	}()
	it.awaitLockWaits(1)
	cancel()
	if err := <-ran; !errors.Is(err, context.Canceled) {
		t.Fatalf("ingest() error = %v, want the cancel of its caller", err)
	}
	if err := holder.Rollback(t.Context()); err != nil {
		t.Fatalf("releasing the mailbox: %v", err)
	}
	it.ingester.now = func() time.Time { return ingestNow.Add(defaultRetryWindow + time.Minute) }

	outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if mails := it.mails(); outcome != outcomeStored || mails != 1 {
		t.Errorf("ingest() past the retry window = %q with %d mails, want %q and the mail stored", outcome, mails,
			outcomeStored)
	}
	if states := it.queueStates(); len(states) != 1 || states[0] != "deleting" {
		t.Errorf("the queue holds %q, want only the raw copy of the cancelled run queued for deletion", states)
	}
}

func TestIngestStopsRecordingAFailureAtTheCleanupTimeout(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	record(t, it.pool, it.box, "file:quote", temporaryDatabaseError, ingestNow.Add(-time.Hour))
	holder := beginTx(t, it.pool)
	if _, err := holder.Exec(t.Context(), "SELECT 1 FROM plugin_email.failed_mail WHERE provider_ref = $1 FOR UPDATE",
		"file:quote"); err != nil {
		t.Fatalf("locking the failure: %v", err)
	}
	it.refuseNewMail("22023")
	it.ingester.times.cleanupTimeout = 200 * time.Millisecond
	ran := make(chan error, 1)
	go func() {
		_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))
		ran <- err
	}()

	select {
	case err := <-ran:
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(fmt.Sprint(err), "email: set the mail aside: ") {
			t.Errorf("ingest() error = %v, want the record of the failure ended at the clean up timeout", err)
		}
	case <-time.After(rivalBudget):
		t.Errorf("ingest() still records the failure after %v, want it ended at the clean up timeout", rivalBudget)
	}
}

func TestSetAsideKeepsTheFilesOfAMailWhoseCommitLanded(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	mail, row, kept := it.prepareMail(invoiceMail("Invoice for March"), "file:invoice", false)
	if _, err := it.ingester.commitMail(t.Context(), sdk.DefaultTenantID, mail, row, kept); err != nil {
		t.Fatalf("commitMail() error = %v, want nil", err)
	}

	_, err := it.ingester.setAside(t.Context(), sdk.DefaultTenantID, mail, kept, context.Canceled)
	it.sweep()

	if !errors.Is(err, context.Canceled) {
		t.Errorf("setAside() error = %v, want the failure it is handed", err)
	}
	if states := it.queueStates(); len(states) != 0 {
		t.Errorf("the queue holds %q, want no file of the stored mail queued for deletion", states)
	}
	it.wantFilesKept(kept)
}

func TestSetAsideKeepsTheFilesOfAMailWhoseCommitLandsWhileItQueuesThem(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	mail, row, kept := it.prepareMail(invoiceMail("Invoice for March"), "file:invoice", false)
	tx := beginTx(t, it.pool)
	if _, err := it.ingester.writeMail(t.Context(), tx, sdk.DefaultTenantID, mail, row, kept); err != nil {
		t.Fatalf("writeMail() error = %v, want nil", err)
	}
	queued := make(chan error, 1)
	go func() {
		_, err := it.ingester.setAside(t.Context(), sdk.DefaultTenantID, mail, kept, context.Canceled)
		queued <- err
	}()
	it.awaitLockWaits(1)

	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("committing the mail while its files are being queued: %v", err)
	}
	if err := <-queued; !errors.Is(err, context.Canceled) {
		t.Errorf("setAside() error = %v, want the failure it is handed", err)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting", "deleting"}) {
		t.Fatalf("the queue holds %q, want both files queued by a clean up that read before the commit", states)
	}
	it.sweep()

	if states := it.queueStates(); len(states) != 0 {
		t.Errorf("the queue holds %q after the sweep, want no row left", states)
	}
	it.wantFilesKept(kept)
}

func TestIngestLogsAQueueItCannotWriteAndStillSetsTheMailAside(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.refuseNewMail("22023")
	it.exec("ALTER TABLE plugin_email.pending_files ADD CONSTRAINT refuse_deleting CHECK (state <> 'deleting') NOT VALID")

	outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if outcome != outcomeFailed {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeFailed)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"writing"}) {
		t.Errorf("the queue holds %q, want the run's row left writing for the sweep", states)
	}
	lines := loggedLines(t, it.logged)
	if len(lines) != 1 || lines[0]["level"] != "WARN" || lines[0]["mailbox"] != it.box.id.String() ||
		lines[0]["reason"] != permanentDatabaseError {
		t.Errorf("ingest() logged %v, want one warning naming the mailbox and the reason", lines)
	}
}

func TestIngestAnswersFailedForAMailSetAsideAtItsReference(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name     string
		reason   string
		failedAt time.Time
		want     Outcome
	}{
		{name: "a permanent error a minute ago", reason: permanentDatabaseError,
			failedAt: ingestNow.Add(-time.Minute), want: outcomeFailed},
		{name: "a temporary error past the retry window", reason: temporaryDatabaseError,
			failedAt: ingestNow.Add(-defaultRetryWindow - time.Minute), want: outcomeFailed},
		{name: "a temporary error inside the retry window", reason: temporaryDatabaseError,
			failedAt: ingestNow.Add(-time.Hour), want: outcomeStored},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			record(t, it.pool, it.box, "file:quote", c.reason, c.failedAt)

			outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

			if outcome != c.want {
				t.Errorf("ingest() = %q, want %q", outcome, c.want)
			}
			failures := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.failed_mail WHERE provider_ref = $1",
				"file:quote")
			if c.want == outcomeFailed && (failures != 1 || len(it.matcher.calls()) != 0) {
				t.Errorf("the mail set aside left %d failures and was matched %d times, want its failure kept and no "+
					"read", failures, len(it.matcher.calls()))
			}
			if c.want == outcomeStored && failures != 0 {
				t.Errorf("the stored mail left %d failures, want its failure removed", failures)
			}
		})
	}
}

func TestIngestRemovesTheFailureOfAMailItLinksToAStoredCopy(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:inbox"))
	record(t, it.pool, it.box, "file:archive", temporaryDatabaseError, ingestNow.Add(-time.Hour))

	outcome := it.ingest(it.incoming(lampMail(receivedLine), "file:archive"))

	if outcome != outcomeDuplicate {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeDuplicate)
	}
	if failures := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.failed_mail WHERE provider_ref = $1",
		"file:archive"); failures != 0 {
		t.Errorf("the linked copy left %d failures, want its failure removed", failures)
	}
}

func TestIngestAnswersDuplicateForAStoredMailAndDeletesTheFailureItLeftBehind(t *testing.T) {
	t.Parallel()

	for name, leave := range map[string]func(it *ingestTest){
		"the temporary failure of a commit whose answer was lost": func(it *ingestTest) {
			mail, row, kept := it.prepareMail(lampMail(), "file:quote", false)
			if _, err := it.ingester.commitMail(it.t.Context(), sdk.DefaultTenantID, mail, row, kept); err != nil {
				it.t.Fatalf("commitMail() error = %v, want nil", err)
			}
			if _, err := it.ingester.setAside(it.t.Context(), sdk.DefaultTenantID, mail, kept,
				context.Canceled); !errors.Is(err, context.Canceled) {
				it.t.Fatalf("setAside() error = %v, want %v", err, context.Canceled)
			}
		},
		"a permanent failure that would set the mail aside": func(it *ingestTest) {
			it.ingest(it.incoming(lampMail(), "file:quote"))
			record(it.t, it.pool, it.box, "file:quote", permanentDatabaseError, ingestNow)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			leave(it)
			heldFailure(t, it.pool, it.box, "file:quote")

			outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

			if outcome != outcomeDuplicate {
				t.Errorf("ingest() = %q, want %q", outcome, outcomeDuplicate)
			}
			if failures := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.failed_mail WHERE provider_ref = $1",
				"file:quote"); failures != 0 {
				t.Errorf("the stored mail left %d failures, want the failure of its reference deleted", failures)
			}
			if mails, matches := it.mails(), len(it.matcher.calls()); mails != 1 || matches != 1 {
				t.Errorf("the workspace holds %d mails after %d matches, want 1 of each and no read of the second run",
					mails, matches)
			}
		})
	}
}

func TestIngestReportsAFailureItCannotDeleteForAStoredMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	it.exec("DROP TABLE plugin_email.failed_mail")

	outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if outcome != "" || pgCode(err) != "42P01" {
		t.Errorf("ingest() = %q, %v, want no outcome and the missing failed_mail table", outcome, err)
	}
}

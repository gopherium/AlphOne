// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// failingRef is the provider reference of the mail the failure tests set aside.
const failingRef = "file:failing"

// retryWindow is how long a mail may keep failing in the failure tests.
const retryWindow = 6 * time.Hour

// failingMailbox returns a pool over a migrated database whose default workspace holds a mailbox, beside the mailbox.
func failingMailbox(t *testing.T) (*pgxpool.Pool, mailbox) {
	t.Helper()
	pool := newMigratedPool(t)
	return pool, storeMailbox(t, pool, sdk.DefaultTenantID)
}

// record sets the mail ref names aside in the default workspace for reason, failed at failedAt.
func record(t *testing.T, db statementRunner, box mailbox, ref, reason string, failedAt time.Time) {
	t.Helper()
	if err := recordFailure(t.Context(), db, sdk.DefaultTenantID, box.id, ref, reason, failedAt); err != nil {
		t.Fatalf("recordFailure() error = %v, want nil", err)
	}
}

// heldFailure reads the failure of the mail ref names in the default workspace, failing the test when there is none.
func heldFailure(t *testing.T, db rowQuerier, box mailbox, ref string) failedMail {
	t.Helper()
	failure, found, err := readFailedMail(t.Context(), db, sdk.DefaultTenantID, box.id, ref)
	if err != nil || !found {
		t.Fatalf("readFailedMail() = %+v, %t, %v, want the failure", failure, found, err)
	}
	return failure
}

func TestMigrateCreatesTheFailedMailTable(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	if !tableExists(t, pool, "plugin_email.failed_mail") {
		t.Error("migrate() left out plugin_email.failed_mail, want it created")
	}
}

func TestTheFailedMailTableTakesTheThreeReasonsOnly(t *testing.T) {
	t.Parallel()

	pool, box := failingMailbox(t)
	for _, reason := range []string{permanentDatabaseError, temporaryDatabaseError, fileStoreError} {
		record(t, pool, box, "file:"+reason, reason, mailReceived)
	}

	err := recordFailure(t.Context(), pool, sdk.DefaultTenantID, box.id, failingRef, "database", mailReceived)

	if code := pgCode(err); code != checkViolation {
		t.Errorf("recordFailure() with the reason database error = %v, want the check to refuse it", err)
	}
}

func TestFailureReasonNamesWhatFailed(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		err  error
		want string
	}{
		"a value the database refuses": {&pgconn.PgError{Code: "22P02"}, permanentDatabaseError},
		"a broken check":               {&pgconn.PgError{Code: checkViolation}, permanentDatabaseError},
		"a broken unique key":          {&pgconn.PgError{Code: "23505"}, permanentDatabaseError},
		"a wrapped broken check": {
			fmt.Errorf("email: insert message: %w", &pgconn.PgError{Code: checkViolation}), permanentDatabaseError,
		},
		"a lost serialization":       {&pgconn.PgError{Code: "40001"}, temporaryDatabaseError},
		"a lost connection":          {&pgconn.PgError{Code: "08006"}, temporaryDatabaseError},
		"a server that restarts":     {&pgconn.PgError{Code: "57P03"}, temporaryDatabaseError},
		"a cancelled caller":         {context.Canceled, temporaryDatabaseError},
		"a mail already stored":      {errAlreadyStored, temporaryDatabaseError},
		"a run the sweep took":       {errFilesSwept, temporaryDatabaseError},
		"a file store that is down":  {fmt.Errorf("%w: %w", errFileStore, errStoreDown), fileStoreError},
		"a file store over postgres": {fmt.Errorf("%w: %w", errFileStore, &pgconn.PgError{Code: "23505"}), fileStoreError},
	} {
		if got := failureReason(test.err); got != test.want {
			t.Errorf("failureReason() of %s = %q, want %q", name, got, test.want)
		}
	}
}

func TestRecordFailureKeepsTheReferenceAndTheReasonOnly(t *testing.T) {
	t.Parallel()

	pool, box := failingMailbox(t)
	reason := failureReason(fmt.Errorf("refused maria.perez@example.com: %w", &pgconn.PgError{Code: checkViolation}))

	record(t, pool, box, failingRef, reason, mailReceived)

	var held string
	if err := pool.QueryRow(t.Context(), `SELECT (to_jsonb(f) - 'first_failed_at' - 'last_failed_at')::text
		FROM plugin_email.failed_mail f`).Scan(&held); err != nil {
		t.Fatalf("reading the failure: %v", err)
	}
	columns := map[string]any{}
	if err := json.Unmarshal([]byte(held), &columns); err != nil {
		t.Fatalf("decoding the failure %s: %v", held, err)
	}
	want := map[string]any{
		"tenant_id": sdk.DefaultTenantID.String(), "mailbox_id": box.id.String(), "provider_ref": failingRef,
		"reason": permanentDatabaseError, "attempts": float64(1),
	}
	if !reflect.DeepEqual(columns, want) {
		t.Errorf("the failure holds %v beside its two times, want only the reference, a fixed reason and the attempts %v",
			columns, want)
	}
	if failure := heldFailure(t, pool, box, failingRef); !failure.firstFailedAt.Equal(mailReceived) ||
		!failure.lastFailedAt.Equal(mailReceived) {
		t.Errorf("the failure = %+v, want it first and last failed at %v", failure, mailReceived)
	}
	if strings.Contains(held, "@") {
		t.Errorf("the failure %s names an address, want none", held)
	}
}

func TestRecordFailureCountsEachAttemptAndKeepsTheFirstTime(t *testing.T) {
	t.Parallel()

	pool, box := failingMailbox(t)
	record(t, pool, box, failingRef, temporaryDatabaseError, mailReceived)

	record(t, pool, box, failingRef, permanentDatabaseError, mailReceived.Add(time.Hour))

	want := failedMail{
		reason: permanentDatabaseError, firstFailedAt: mailReceived, lastFailedAt: mailReceived.Add(time.Hour), attempts: 2,
	}
	if got := heldFailure(t, pool, box, failingRef); !reflect.DeepEqual(got, want) {
		t.Errorf("the failure after two attempts = %+v, want %+v", got, want)
	}
}

func TestRecordFailureKeepsEachReferenceAndWorkspaceApart(t *testing.T) {
	t.Parallel()

	pool, box := failingMailbox(t)
	acme := storeWorkspace(t, pool)
	acmeBox := storeMailbox(t, pool, acme)
	record(t, pool, box, failingRef, temporaryDatabaseError, mailReceived)
	record(t, pool, box, "file:other", temporaryDatabaseError, mailReceived)
	if err := recordFailure(t.Context(), pool, acme, acmeBox.id, failingRef, temporaryDatabaseError,
		mailReceived); err != nil {
		t.Fatalf("recordFailure() in Acme error = %v, want nil", err)
	}

	if attempts := heldFailure(t, pool, box, failingRef).attempts; attempts != 1 {
		t.Errorf("the failure counts %d attempts, want 1, each reference and workspace apart", attempts)
	}
	if rows := countRows(t, pool, "SELECT count(*) FROM plugin_email.failed_mail WHERE attempts = $1", 1); rows != 3 {
		t.Errorf("failed_mail holds %d rows of one attempt, want 3", rows)
	}
}

func TestReadFailedMailAnswersNoneForAMailThatNeverFailed(t *testing.T) {
	t.Parallel()

	pool, box := failingMailbox(t)

	failure, found, err := readFailedMail(t.Context(), pool, sdk.DefaultTenantID, box.id, failingRef)

	if err != nil || found || failure != (failedMail{}) {
		t.Errorf("readFailedMail() = %+v, %t, %v, want none", failure, found, err)
	}
}

func TestReadFailedMailLeavesOutAnotherWorkspacesFailure(t *testing.T) {
	t.Parallel()

	pool, box := failingMailbox(t)
	record(t, pool, box, failingRef, permanentDatabaseError, mailReceived)

	_, found, err := readFailedMail(t.Context(), pool, storeWorkspace(t, pool), box.id, failingRef)

	if err != nil || found {
		t.Errorf("readFailedMail() in Acme = %t, %v, want none", found, err)
	}
}

func TestIsSetAsideAtOnceForAPermanentErrorAndPastTheWindowForAnyOther(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		reason string
		after  time.Duration
		want   bool
	}{
		"a permanent error at once":            {permanentDatabaseError, 0, true},
		"a temporary error at once":            {temporaryDatabaseError, 0, false},
		"a temporary error at the window":      {temporaryDatabaseError, retryWindow, false},
		"a temporary error past the window":    {temporaryDatabaseError, retryWindow + time.Second, true},
		"a file store error past the window":   {fileStoreError, retryWindow + time.Second, true},
		"a file store error inside the window": {fileStoreError, time.Hour, false},
	} {
		failure := failedMail{reason: test.reason, firstFailedAt: mailReceived, lastFailedAt: mailReceived, attempts: 1}

		if got := failure.isSetAside(mailReceived.Add(test.after), retryWindow); got != test.want {
			t.Errorf("isSetAside() of %s = %t, want %t", name, got, test.want)
		}
	}
}

func TestDeleteFailedMailRemovesOnlyItsReference(t *testing.T) {
	t.Parallel()

	pool, box := failingMailbox(t)
	record(t, pool, box, failingRef, temporaryDatabaseError, mailReceived)
	record(t, pool, box, "file:other", temporaryDatabaseError, mailReceived)
	tx := beginTx(t, pool)

	if err := deleteFailedMail(t.Context(), tx, sdk.DefaultTenantID, box.id, failingRef); err != nil {
		t.Fatalf("deleteFailedMail() error = %v, want nil", err)
	}

	if _, found, err := readFailedMail(t.Context(), tx, sdk.DefaultTenantID, box.id, failingRef); err != nil || found {
		t.Errorf("readFailedMail() after the delete = %t, %v, want none", found, err)
	}
	heldFailure(t, tx, box, "file:other")
}

func TestFailureStatementsReportAnEndedTransaction(t *testing.T) {
	t.Parallel()

	tx := endedTx(t)
	ctx, id := t.Context(), uuid.Must(uuid.NewV7())
	for prefix, call := range map[string]func() error{
		"email: set the mail aside: ": func() error {
			return recordFailure(ctx, tx, sdk.DefaultTenantID, id, failingRef, temporaryDatabaseError, mailReceived)
		},
		"email: read the failure of a mail: ": func() error {
			_, _, err := readFailedMail(ctx, tx, sdk.DefaultTenantID, id, failingRef)
			return err
		},
		"email: delete the failure of a mail: ": func() error {
			return deleteFailedMail(ctx, tx, sdk.DefaultTenantID, id, failingRef)
		},
	} {
		err := call()

		if !errors.Is(err, pgx.ErrTxClosed) || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("a statement on an ended transaction error = %v, want %q naming %v", err, prefix, pgx.ErrTxClosed)
		}
	}
}

// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

func TestIngestAnswersWorkspaceClosedAndReadsNothing(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.gate.closed = true

	outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if outcome != outcomeWorkspaceClosed {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeWorkspaceClosed)
	}
	if steps := it.log.steps(); !slices.Equal(steps, []string{"gate"}) {
		t.Errorf("the host saw %q, want the gate alone", steps)
	}
	if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the workspace holds %d rows of mail, want none", rows)
	}
}

func TestIngestReportsAGateThatFails(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.gate.err = errGateDown

	outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errGateDown) || outcome != "" {
		t.Errorf("ingest() = %q, %v, want no outcome and %v", outcome, err, errGateDown)
	}
}

func TestIngestReportsAMailboxThatDoesNotExist(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	in := it.incoming(lampMail(), "file:quote")
	in.MailboxID = uuid.Must(uuid.NewV7())

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, in)

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("ingest() error = %v, want %v", err, pgx.ErrNoRows)
	}
}

func TestIngestNeedsTheTimeTheMailboxReceivedTheMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	in := it.incoming(lampMail(), "file:quote")
	in.ReceivedAt = time.Time{}

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, in)

	if !errors.Is(err, errNoReceivedTime) {
		t.Errorf("ingest() error = %v, want %v", err, errNoReceivedTime)
	}
	if steps := it.log.steps(); len(steps) != 0 {
		t.Errorf("the host saw %q, want nothing asked", steps)
	}
}

func TestIngestReportsARawMailItCannotRead(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		raw  io.Reader
	}{
		{name: "at once", raw: failingReader{}},
		{name: "past the message cap", raw: io.MultiReader(bytes.NewReader(lampMail()), failingReader{})},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t, func(s *settings) { s.parse.MaxBytes = 100 })
			in := it.incoming(nil, "file:quote")
			in.Raw = c.raw

			_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, in)

			if !errors.Is(err, errEntropy) {
				t.Errorf("ingest() error = %v, want %v", err, errEntropy)
			}
			if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
				t.Errorf("the workspace holds %d rows of mail, want none for a read the caller retries", rows)
			}
		})
	}
}

func TestIngestReportsAMatchThatFails(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.matcher.err = errMatchDown

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errMatchDown) {
		t.Errorf("ingest() error = %v, want %v", err, errMatchDown)
	}
	if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the workspace holds %d rows of mail, want none", rows)
	}
}

func TestIngestReportsAWorkspaceLimitItCannotRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingester.files.settings = failingSettings{}

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errSettingsDown) {
		t.Errorf("ingest() error = %v, want %v", err, errSettingsDown)
	}
	if rows := it.rows(sdk.DefaultTenantID); rows != 0 || len(it.store.savedTypes()) != 0 {
		t.Errorf("the workspace holds %d rows of mail and %d saved files, want none", rows, len(it.store.savedTypes()))
	}
}

func TestIngestReportsAStoredMailItCannotLookUp(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	it.matcher.hook = cancel

	_, err := it.ingester.ingest(ctx, sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, context.Canceled) {
		t.Errorf("ingest() error = %v, want %v", err, context.Canceled)
	}
}

func TestIngestReportsAFailureItCannotRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.exec("DROP TABLE plugin_email.failed_mail")

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if pgCode(err) != "42P01" {
		t.Errorf("ingest() error = %v, want the missing failed_mail table", err)
	}
}

func TestWriteMailReportsEachStatementOfANewMailThatFails(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{
		"FOR NO KEY UPDATE", "SELECT message_id FROM plugin_email.provider_refs", "AND dedupe_key = $3",
		"FROM plugin_email.thread_keys k", "INSERT INTO plugin_email.messages",
		"INSERT INTO plugin_email.provider_refs", "INSERT INTO plugin_email.message_contacts",
		"INSERT INTO plugin_email.attachments", "INSERT INTO plugin_email.thread_keys",
		"SELECT message_id FROM plugin_email.thread_keys", "UPDATE plugin_email.threads SET",
		"DELETE FROM plugin_email.pending_files", "DELETE FROM plugin_email.failed_mail",
		"INSERT INTO plugin_email.workspace_usage",
	} {
		t.Run(fragment, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			mail, row, kept := it.prepareMail(invoiceMail("Invoice for March"), "file:invoice", false)
			tx := batchFailingTx{failingTx{Tx: beginTx(t, it.pool), fragment: fragment}}

			_, err := it.ingester.writeMail(t.Context(), tx, sdk.DefaultTenantID, mail, row, kept)

			if pgCode(err) != undefinedFunction {
				t.Errorf("writeMail() error = %v, want the injected failure", err)
			}
		})
	}
}

func TestWriteMailReportsEachStatementOfALinkedCopyThatFails(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{
		"INSERT INTO plugin_email.provider_refs", "INSERT INTO plugin_email.message_contacts", "SET participants",
		"SET direction = 'outbound'", "'deleting', queued.size", "DELETE FROM plugin_email.failed_mail",
	} {
		t.Run(fragment, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			it.ingest(it.incoming(infoMail(), "file:inbox"))
			mail, row, kept := it.prepareMail(infoMail(receivedLine), "file:sent", true)
			tx := batchFailingTx{failingTx{Tx: beginTx(t, it.pool), fragment: fragment}}

			_, err := it.ingester.writeMail(t.Context(), tx, sdk.DefaultTenantID, mail, row, kept)

			if pgCode(err) != undefinedFunction {
				t.Errorf("writeMail() error = %v, want the injected failure", err)
			}
		})
	}
}

func TestWriteCopyReportsEachStatementThatFails(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{
		"FOR NO KEY UPDATE", "SELECT message_id FROM plugin_email.provider_refs", "AND dedupe_key = $3",
		"INSERT INTO plugin_email.provider_refs",
	} {
		t.Run(fragment, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			it.ingest(it.incoming(lampMail(), "file:inbox"))
			mail, err := it.ingester.readIncoming(t.Context(), it.box, it.incoming(lampMail(receivedLine), "file:archive"))
			if err != nil {
				t.Fatalf("readIncoming() error = %v, want nil", err)
			}
			tx := failingTx{Tx: beginTx(t, it.pool), fragment: fragment}

			_, err = it.ingester.writeCopy(t.Context(), tx, sdk.DefaultTenantID, mail)

			if pgCode(err) != undefinedFunction {
				t.Errorf("writeCopy() error = %v, want the injected failure", err)
			}
		})
	}
}

func TestCommitReportsATransactionItCannotBegin(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	mail, row, kept := it.prepareMail(lampMail(), "file:quote", false)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := it.ingester.commitMail(ctx, sdk.DefaultTenantID, mail, row, kept); !errors.Is(err, context.Canceled) {
		t.Errorf("commitMail() error = %v, want %v", err, context.Canceled)
	}
	if _, err := it.ingester.commitCopy(ctx, sdk.DefaultTenantID, mail); !errors.Is(err, context.Canceled) {
		t.Errorf("commitCopy() error = %v, want %v", err, context.Canceled)
	}
}

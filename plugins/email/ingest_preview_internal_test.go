// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

func TestPreviewAnswersTheOutcomeAMailWouldReachAndWritesNothing(t *testing.T) {
	t.Parallel()

	storeLamp := func(it *ingestTest) { it.ingest(it.incoming(lampMail(), "file:inbox")) }
	for _, c := range []struct {
		name    string
		prepare func(*ingestTest)
		raw     []byte
		ref     string
		want    Outcome
	}{
		{name: "a new mail", raw: lampMail(), ref: "file:inbox", want: outcomeStored},
		{name: "a mail too broken to read", raw: brokenMail("maria.perez@example.com"), ref: "file:broken",
			want: outcomeStoredHeadersOnly},
		{name: "a mail from a stranger", raw: strangerMail(), ref: "file:offer", want: outcomeNoKnownContact},
		{name: "a broken mail from a stranger", raw: brokenMail("stranger@example.com"), ref: "file:broken",
			want: outcomeHeadersOnlyNoKnownContact},
		{name: "a redelivery", prepare: storeLamp, raw: lampMail(), ref: "file:inbox", want: outcomeDuplicate},
		{name: "another copy", prepare: storeLamp, raw: lampMail(receivedLine), ref: "file:archive",
			want: outcomeDuplicate},
		{name: "another copy naming another contact", prepare: storeLamp,
			raw: lampMail("Cc: Ada Lovelace <ada@example.com>"), ref: "file:archive", want: outcomeLinked},
		{name: "a mail set aside", prepare: func(it *ingestTest) {
			record(it.t, it.pool, it.box, "file:inbox", permanentDatabaseError, ingestNow)
		}, raw: lampMail(), ref: "file:inbox", want: outcomeFailed},
		{name: "a stored mail whose failure stayed", prepare: func(it *ingestTest) {
			storeLamp(it)
			record(it.t, it.pool, it.box, "file:inbox", permanentDatabaseError, ingestNow)
		}, raw: lampMail(), ref: "file:inbox", want: outcomeDuplicate},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			if c.prepare != nil {
				c.prepare(it)
			}
			rows, saves, events := it.rows(sdk.DefaultTenantID), len(it.store.savedTypes()), len(it.events())

			outcome, err := it.ingester.preview(t.Context(), sdk.DefaultTenantID, it.box, it.incoming(c.raw, c.ref))

			if err != nil || outcome != c.want {
				t.Errorf("preview() = %q, %v, want %q", outcome, err, c.want)
			}
			if it.rows(sdk.DefaultTenantID) != rows || len(it.store.savedTypes()) != saves || len(it.events()) != events {
				t.Error("preview() wrote a row, saved a file or published an event, want nothing written")
			}
		})
	}
}

func TestPreviewMatchesTheOwnAddressesOfAMailboxNotYetStored(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	acme := storeWorkspace(t, it.pool)
	it.knowContacts(acme)
	box := mailbox{address: "info@example.com", aliases: []string{"sales@example.com"}}

	outcome, err := it.ingester.preview(t.Context(), acme, box, it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: sales@example.com", "Cc: info@example.com",
		"Subject: A second lamp", "Content-Type: text/plain", "", lampText), "file:second-lamp"))

	if err != nil || outcome != outcomeStored {
		t.Errorf("preview() = %q, %v, want %q", outcome, err, outcomeStored)
	}
	if batches := it.matcher.calls(); !reflect.DeepEqual(batches, [][]string{{"maria.perez@example.com"}}) {
		t.Errorf("the matcher was asked %q, want Maria Perez alone, the address and the alias left out", batches)
	}
	if mailboxes := mailboxRows(t, it.pool, acme); mailboxes != 0 || it.rows(acme) != 0 {
		t.Errorf("Acme holds %d mailboxes and %d rows of mail, want none", mailboxes, it.rows(acme))
	}
}

func TestPreviewAnswersAClosedWorkspaceAndAGateThatFails(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.gate.closed = true

	outcome, err := it.ingester.preview(t.Context(), sdk.DefaultTenantID, it.box, it.incoming(lampMail(), "file:quote"))

	if err != nil || outcome != outcomeWorkspaceClosed {
		t.Errorf("preview() = %q, %v, want %q", outcome, err, outcomeWorkspaceClosed)
	}

	it.gate.err = errGateDown

	if _, err := it.ingester.preview(t.Context(), sdk.DefaultTenantID, it.box,
		it.incoming(lampMail(), "file:quote")); !errors.Is(err, errGateDown) {
		t.Errorf("preview() error = %v, want %v", err, errGateDown)
	}
}

func TestPreviewReportsEachReadThatFails(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name    string
		prepare func(it *ingestTest, cancel context.CancelFunc)
		want    error
		code    string
	}{
		{name: "the provider reference", prepare: func(it *ingestTest, cancel context.CancelFunc) {
			it.gate.hook = cancel
		}, want: context.Canceled},
		{name: "the failure", prepare: func(it *ingestTest, _ context.CancelFunc) {
			it.exec("DROP TABLE plugin_email.failed_mail")
		}, code: "42P01"},
		{name: "the stored mail", prepare: func(it *ingestTest, cancel context.CancelFunc) {
			it.matcher.hook = cancel
		}, want: context.Canceled},
		{name: "the new links", prepare: func(it *ingestTest, _ context.CancelFunc) {
			it.ingest(it.incoming(lampMail(), "file:inbox"))
			it.exec("DROP TABLE plugin_email.message_contacts")
		}, code: "42P01"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.prepare(it, cancel)

			_, err := it.ingester.preview(ctx, sdk.DefaultTenantID, it.box, it.incoming(lampMail(receivedLine),
				"file:archive"))

			if c.want != nil && !errors.Is(err, c.want) || c.code != "" && pgCode(err) != c.code {
				t.Errorf("preview() error = %v, want %v%s", err, c.want, c.code)
			}
		})
	}
}

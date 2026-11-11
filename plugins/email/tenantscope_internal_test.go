// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// addWorkspace stores the workspace Acme with a mailbox and known contacts of its own, returning both.
func (it *ingestTest) addWorkspace() (uuid.UUID, mailbox) {
	it.t.Helper()
	acme := storeWorkspace(it.t, it.pool)
	box := storeMailbox(it.t, it.pool, acme)
	it.knowContacts(acme)
	return acme, box
}

// ingestIn ingests raw under ref into the mailbox box of the workspace tenant and fails the test on an error.
func (it *ingestTest) ingestIn(tenant uuid.UUID, box mailbox, raw []byte, ref string) Outcome {
	it.t.Helper()
	in := it.incoming(raw, ref)
	in.MailboxID = box.id
	outcome, err := it.ingester.ingest(it.t.Context(), tenant, in)
	if err != nil {
		it.t.Fatalf("ingest() error = %v, want nil", err)
	}
	return outcome
}

// linksIn returns the contacts the mail of the workspace tenant is linked to.
func (it *ingestTest) linksIn(tenant uuid.UUID) []uuid.UUID {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(),
		"SELECT contact_id FROM plugin_email.message_contacts WHERE tenant_id = $1 ORDER BY contact_id", tenant)
	contacts, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		it.t.Fatalf("reading the links of a workspace: %v", err)
	}
	return contacts
}

func TestIngestKeepsTheMailKeysAndDedupeOfEachWorkspaceApart(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	acme, box := it.addWorkspace()

	first := it.ingestIn(sdk.DefaultTenantID, it.box, lampMail(), "file:quote")
	second := it.ingestIn(acme, box, lampMail(), "file:quote")

	if first != outcomeStored || second != outcomeStored {
		t.Errorf("one mail fed to two workspaces answered %q and %q, want stored in each", first, second)
	}
	for tenant, contact := range map[uuid.UUID]uuid.UUID{
		sdk.DefaultTenantID: it.contacts["maria.perez@example.com"],
		acme:                it.matcher.known[acme]["maria.perez@example.com"],
	} {
		mails := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1", tenant)
		keys := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.thread_keys WHERE tenant_id = $1", tenant)
		if mails != 1 || keys != 1 {
			t.Errorf("the workspace %v holds %d mails and %d thread keys, want 1 of each", tenant, mails, keys)
		}
		if links := it.linksIn(tenant); !slices.Equal(links, []uuid.UUID{contact}) {
			t.Errorf("the mail of the workspace %v is linked to %v, want its own Maria Perez %v", tenant, links, contact)
		}
	}
}

func TestOneMessageIDInTwoWorkspacesGivesTwoRowsEachHoldingItsOwnKey(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	acme, box := it.addWorkspace()

	it.ingestIn(sdk.DefaultTenantID, it.box, lampMail(), "file:quote")
	it.ingestIn(acme, box, quoteMail(nil, "A different text under the same id."), "file:other")

	if rows := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.messages WHERE internet_message_id = $1",
		"quote@example.com"); rows != 2 {
		t.Errorf("the id quote@example.com names %d rows, want one in each workspace", rows)
	}
	if held := countRows(t, it.pool, `SELECT count(*) FROM plugin_email.thread_keys k
		JOIN plugin_email.messages m ON m.id = k.message_id AND m.tenant_id = k.tenant_id
		WHERE k.kind = $1`, internetMessageIDKind); held != 2 {
		t.Errorf("%d mails hold the key of their own id, want each workspace's mail to hold its own", held)
	}
	if logged := it.logged.String(); strings.Contains(logged, "holder") {
		t.Errorf("ingest() logged %q, want no id reused across workspaces", logged)
	}
}

func TestIngestNeverLinksAContactOfAnotherWorkspace(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	acme, box := it.addWorkspace()
	foreign := it.contacts["maria.perez@example.com"]
	it.matcher.known[acme] = map[string]uuid.UUID{"maria.perez@example.com": foreign}

	outcome := it.ingestIn(acme, box, lampMail(), "file:quote")

	if outcome != outcomeStored {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeStored)
	}
	if links := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.message_contacts WHERE contact_id = $1",
		foreign); links != 0 {
		t.Errorf("the contact of the default workspace holds %d links, want none from Acme's mail", links)
	}
}

func TestIngestServesTheWorkspaceItIsHandedWhateverItsContextNames(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		ctx  func(context.Context) context.Context
	}{
		{name: "a context naming no workspace", ctx: func(ctx context.Context) context.Context { return ctx }},
		{name: "a context naming another workspace", ctx: func(ctx context.Context) context.Context {
			return sdk.WithTenant(ctx, sdk.DefaultTenantID)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			acme, box := it.addWorkspace()
			in := it.incoming(invoiceMail("Invoice for March"), "file:invoice")
			in.MailboxID = box.id

			outcome, err := it.ingester.ingest(c.ctx(t.Context()), acme, in)

			if err != nil || outcome != outcomeStored {
				t.Fatalf("ingest() = %q, %v, want %q", outcome, err, outcomeStored)
			}
			for _, call := range it.log.all() {
				if call.tenant != acme {
					t.Errorf("the host saw the %s call in the workspace %v, want Acme %v", call.step, call.tenant, acme)
				}
			}
			if events := it.events(); len(events) != 1 || events[0].tenant != acme {
				t.Errorf("the events are %+v, want one in Acme", events)
			}
			if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
				t.Errorf("the default workspace holds %d rows of mail, want none", rows)
			}
			row := storedRow(t, it.pool, it.mailTitled("Invoice"))
			if kept := storedBytes(t, it.store, acme, row.rawFileID); len(kept) == 0 {
				t.Error("Acme keeps no raw copy, want the file saved in Acme")
			}
		})
	}
}

func TestTheSweepDeletesTheFilesOfAFailedIngestInTheWorkspaceItServed(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	acme, box := it.addWorkspace()
	it.refuseNewMail("22023")
	if outcome := it.ingestIn(acme, box, lampMail(), "file:quote"); outcome != outcomeFailed {
		t.Fatalf("ingest() = %q, want %q", outcome, outcomeFailed)
	}
	var queued uuid.UUID
	if err := it.pool.QueryRow(t.Context(), `SELECT file_id FROM plugin_email.pending_files
		WHERE tenant_id = $1 AND state = 'deleting'`, acme).Scan(&queued); err != nil {
		t.Fatalf("reading the queued file of Acme: %v", err)
	}
	var deletedIn []uuid.UUID
	logger, _ := bufferLogger()
	s := &sweeper{
		pool: it.pool,
		store: deletingStore{FileStore: it.store, before: func(ctx context.Context, _ uuid.UUID) error {
			tenant, _ := sdk.TenantFromContext(ctx)
			deletedIn = append(deletedIn, tenant)
			return nil
		}},
		logger: logger,
		pace:   sweepSettings{interval: time.Hour, batch: 100, budget: time.Minute, orphanAge: orphanAge},
	}

	s.sweep(t.Context())

	if !slices.Equal(deletedIn, []uuid.UUID{acme}) {
		t.Errorf("the sweep deleted in %v, want Acme %v alone", deletedIn, acme)
	}
	if !fileGone(t, it.store, acme, queued) || queueSize(t, it.pool, acme) != 0 {
		t.Error("the file of the failed ingest or its row is kept, want both deleted in Acme")
	}
}

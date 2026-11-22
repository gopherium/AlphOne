// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// costOf returns a change of a test mail that has it add size bytes to its workspace.
func costOf(size int64) func(*messageRow) {
	return func(row *messageRow) { row.storedBytes = size }
}

// usageIn reads the space the workspace tenant uses through the test's ingester, failing the test on an error.
func (it *ingestTest) usageIn(tenant uuid.UUID) workspaceUsage {
	it.t.Helper()
	usage, err := it.ingester.files.readUsage(it.t.Context(), tenant)
	if err != nil {
		it.t.Fatalf("readUsage() error = %v, want nil", err)
	}
	return usage
}

// costliest returns the ids of the mails of the workspace tenant that cost it most, a page of readPage at most.
func (it *ingestTest) costliest(tenant uuid.UUID) []uuid.UUID {
	it.t.Helper()
	mails, err := readCostliestMails(it.t.Context(), it.pool, tenant, readPage)
	if err != nil {
		it.t.Fatalf("readCostliestMails() error = %v, want nil", err)
	}
	return mailIDs(mails)
}

func TestReadUsageCountsTheBytesTheWorkspaceKeepsBesideEveryQueuedFile(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	id := it.mailTitled("Invoice")
	kept := usageOf(t, it.pool, sdk.DefaultTenantID)
	file, size := it.attachmentIDs(id)[0], it.attachmentsOf(id)[0].size
	stored := it.usageIn(sdk.DefaultTenantID)
	if _, err := it.freeFiles(filesDelete{message: id, attachment: &file}); err != nil {
		t.Fatalf("deleteFiles() error = %v, want nil", err)
	}
	freeing := it.usageIn(sdk.DefaultTenantID)
	queueAt(t, it.pool, sdk.DefaultTenantID, uuid.Must(uuid.NewV7()), "writing", 40)
	writing := it.usageIn(sdk.DefaultTenantID)
	it.sweep()

	swept := it.usageIn(sdk.DefaultTenantID)

	for name, c := range map[string]struct {
		got  workspaceUsage
		want workspaceUsage
	}{
		"the stored mail":                {stored, workspaceUsage{usedBytes: kept}},
		"a deleted file before a sweep":  {freeing, workspaceUsage{usedBytes: kept, freeingBytes: size}},
		"a file a run is writing":        {writing, workspaceUsage{usedBytes: kept + 40, freeingBytes: size}},
		"the deleted file after a sweep": {swept, workspaceUsage{usedBytes: kept - size + 40}},
	} {
		c.want.limitBytes = defaultWorkspaceMaxBytes
		if c.got != c.want {
			t.Errorf("readUsage() of %s = %+v, want %+v", name, c.got, c.want)
		}
	}
}

func TestReadUsageAnswersTheOwnLimitOfAWorkspaceTheDefaultOrNone(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		fallback int
		own      string
		want     int64
	}{
		"a workspace that holds no limit of its own": {fallback: 5000, want: 5000},
		"a workspace that holds a limit of its own":  {fallback: 5000, own: "2000000", want: 2000000},
		"a workspace whose own limit is zero":        {fallback: 5000, own: "0", want: 0},
		"an install with no limit":                   {fallback: 0, want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t, func(s *settings) { s.files.workspaceMaxBytes = c.fallback })
			if c.own != "" {
				holdLimit(t, it.ingester.files.settings, sdk.DefaultTenantID, c.own)
			}

			usage := it.usageIn(sdk.DefaultTenantID)

			if usage.limitBytes != c.want {
				t.Errorf("readUsage() limit = %d, want %d", usage.limitBytes, c.want)
			}
		})
	}
}

func TestReadUsageAnswersNothingUsedForAWorkspaceThatHoldsNoMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	noMailbox := storeWorkspace(t, it.pool)

	usage := it.usageIn(noMailbox)

	if want := (workspaceUsage{limitBytes: defaultWorkspaceMaxBytes}); usage != want {
		t.Errorf("readUsage() of a workspace with no mailbox = %+v, want %+v", usage, want)
	}
}

func TestReadUsageAndTheCostliestMailsKeepToTheirWorkspace(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	acme, box := it.addWorkspace()
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	it.ingestIn(acme, box, lampMail(), "file:quote")
	queueAt(t, it.pool, acme, uuid.Must(uuid.NewV7()), "deleting", 70)
	home := it.mailTitled("Invoice")
	inAcme := it.mailTitled("Your quote")

	homeUsage, acmeUsage := it.usageIn(sdk.DefaultTenantID), it.usageIn(acme)

	if homeUsage.usedBytes != usageOf(t, it.pool, sdk.DefaultTenantID) || homeUsage.freeingBytes != 0 {
		t.Errorf("readUsage() of the default workspace = %+v, want its own bytes and nothing of Acme's", homeUsage)
	}
	if acmeUsage.usedBytes != usageOf(t, it.pool, acme)+70 || acmeUsage.freeingBytes != 70 {
		t.Errorf("readUsage() of Acme = %+v, want its own bytes and its 70 bytes freeing", acmeUsage)
	}
	if listed := it.costliest(sdk.DefaultTenantID); !slices.Equal(listed, []uuid.UUID{home}) {
		t.Errorf("readCostliestMails() of the default workspace = %v, want its own %v alone", listed, home)
	}
	if listed := it.costliest(acme); !slices.Equal(listed, []uuid.UUID{inAcme}) {
		t.Errorf("readCostliestMails() of Acme = %v, want its own %v alone", listed, inAcme)
	}
}

func TestReadUsageReportsEachReadThatFails(t *testing.T) {
	t.Parallel()

	limit := newIngestTest(t)
	limit.ingester.files.settings = failingSettings{}
	total := newIngestTest(t)
	total.exec("DROP TABLE plugin_email.pending_files")

	_, limitErr := limit.ingester.files.readUsage(t.Context(), sdk.DefaultTenantID)
	_, totalErr := total.ingester.files.readUsage(t.Context(), sdk.DefaultTenantID)

	if !errors.Is(limitErr, errSettingsDown) {
		t.Errorf("readUsage() over a settings store that fails error = %v, want %v", limitErr, errSettingsDown)
	}
	if pgCode(totalErr) != "42P01" || !strings.HasPrefix(totalErr.Error(), "email: read the space a workspace uses: ") {
		t.Errorf("readUsage() over a missing queue error = %v, want the failed read of the space named", totalErr)
	}
}

func TestReadCostliestMailsListsTheMailByWhatItCostsWithTiesByIDWithinTheLimit(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	small := mailIn(t, tt.tx, tt.box, thread, 0, costOf(200))
	large := mailIn(t, tt.tx, tt.box, thread, time.Hour, costOf(5000))
	tied := byIDDescending(mailIn(t, tt.tx, tt.box, thread, 2*time.Hour, costOf(1300)),
		mailIn(t, tt.tx, tt.box, thread, 3*time.Hour, costOf(1300)))
	want := slices.Concat([]uuid.UUID{large}, tied, []uuid.UUID{small})

	whole, err := readCostliestMails(t.Context(), tt.tx, sdk.DefaultTenantID, readPage)
	page, pageErr := readCostliestMails(t.Context(), tt.tx, sdk.DefaultTenantID, 2)

	if err != nil || !slices.Equal(mailIDs(whole), want) {
		t.Errorf("readCostliestMails() = %v, %v, want %v", mailIDs(whole), err, want)
	}
	if pageErr != nil || !slices.Equal(mailIDs(page), want[:2]) {
		t.Errorf("readCostliestMails() of 2 = %v, %v, want %v", mailIDs(page), pageErr, want[:2])
	}
}

func TestReadCostliestMailsCountsAMailThatKeptNoFileByItsText(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.files.workspaceMaxBytes = 1 })
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	it.ingest(it.incoming(lampMail(), "file:quote"))

	mails, err := readCostliestMails(t.Context(), it.pool, sdk.DefaultTenantID, readPage)

	if err != nil || len(mails) != 2 {
		t.Fatalf("readCostliestMails() = %v, %v, want both mails", mailIDs(mails), err)
	}
	for _, mail := range mails {
		row := storedRow(t, it.pool, mail.id)
		if text := textBytes(t, row); mail.storedBytes != text || text == 0 || row.rawState != fileOverLimit {
			t.Errorf("the mail %q costs %d bytes with its original %q, want its %d bytes of text and no file kept",
				mail.subject, mail.storedBytes, row.rawState, text)
		}
	}
	if mails[0].storedBytes < mails[1].storedBytes {
		t.Errorf("readCostliestMails() answered %d bytes before %d, want the costliest first",
			mails[0].storedBytes, mails[1].storedBytes)
	}
}

func TestADeleteOfFilesMovesTheirMailDownTheCostliestMails(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(invoiceMail("%PDF-1.4 march"), "file:invoice"))
	it.ingest(it.incoming(lampMail(), "file:quote"))
	invoice, quote := it.mailTitled("Invoice"), it.mailTitled("Your quote")
	if before := it.costliest(sdk.DefaultTenantID); !slices.Equal(before, []uuid.UUID{invoice, quote}) {
		t.Fatalf("readCostliestMails() before the delete = %v, want the invoice %v first", before, invoice)
	}
	file := it.attachmentIDs(invoice)[0]

	if _, err := it.freeFiles(filesDelete{message: invoice, attachment: &file, original: true}); err != nil {
		t.Fatalf("deleteFiles() error = %v, want nil", err)
	}

	if after := it.costliest(sdk.DefaultTenantID); !slices.Equal(after, []uuid.UUID{quote, invoice}) {
		t.Errorf("readCostliestMails() after the delete = %v, want the quote %v first", after, quote)
	}
}

func TestReadCostliestMailsReadsThroughTheIndexOfWhatMailCosts(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	mailIn(t, tt.tx, tt.box, thread, 0, costOf(200))
	var sent []string
	if _, err := readCostliestMails(t.Context(), recordingTx{Tx: tt.tx, sent: &sent}, sdk.DefaultTenantID,
		readPage); err != nil || len(sent) != 1 {
		t.Fatalf("readCostliestMails() sent %d statements, %v, want one", len(sent), err)
	}
	if _, err := tt.tx.Exec(t.Context(), "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatalf("turning sequential scans off: %v", err)
	}

	rows, _ := tt.tx.Query(t.Context(), "EXPLAIN "+sent[0], sdk.DefaultTenantID, readPage)
	plan, err := pgx.CollectRows(rows, pgx.RowTo[string])

	if text := strings.Join(plan, "\n"); err != nil || !strings.Contains(text, "messages_stored_bytes_idx") ||
		strings.Contains(text, "Sort") {
		t.Errorf("the plan of readCostliestMails() = %s, %v, want messages_stored_bytes_idx and no sort", text, err)
	}
}

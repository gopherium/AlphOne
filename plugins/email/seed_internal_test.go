// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// seededThreadSubjects reads the subjects of the mails of one thread in the order they were received.
const seededThreadSubjects = `SELECT string_agg(subject, ' | ' ORDER BY received_at, id)
	FROM plugin_email.messages WHERE thread_id = $1`

// seededCounts counts the mailboxes, threads, mails, attachments and contact links of one workspace.
const seededCounts = `SELECT
	(SELECT count(*) FROM plugin_email.mailboxes WHERE tenant_id = $1),
	(SELECT count(*) FROM plugin_email.threads WHERE tenant_id = $1),
	(SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1),
	(SELECT count(*) FROM plugin_email.attachments WHERE tenant_id = $1),
	(SELECT count(*) FROM plugin_email.message_contacts WHERE tenant_id = $1)`

// crossReferenceEntry matches one in use entry of the cross reference table of a PDF.
var crossReferenceEntry = regexp.MustCompile(`(?m)^(\d{10}) 00000 n \n`)

// seededWorkspace is a workspace of an ingest test the seed filled.
type seededWorkspace struct {
	// tenant names the workspace.
	tenant uuid.UUID
	// box is the sample mailbox.
	box mailbox
}

// seedWorkspace stores a workspace knowing every known email and seeds it.
func (it *ingestTest) seedWorkspace() seededWorkspace {
	it.t.Helper()
	tenant := storeWorkspace(it.t, it.pool)
	it.knowContacts(tenant)
	if err := it.ingester.seed(it.t.Context(), tenant); err != nil {
		it.t.Fatalf("seed() error = %v, want nil", err)
	}
	box, held, err := readWorkspaceMailbox(it.t.Context(), it.pool, tenant)
	if err != nil || !held {
		it.t.Fatalf("readWorkspaceMailbox() = %v, %v, want the sample mailbox", held, err)
	}
	return seededWorkspace{tenant: tenant, box: box}
}

// counts reads the mailboxes, threads, mails, attachments and contact links of the seeded workspace.
func (s seededWorkspace) counts(t *testing.T, it *ingestTest) [5]int {
	t.Helper()
	var held [5]int
	if err := it.pool.QueryRow(t.Context(), seededCounts, s.tenant).Scan(
		&held[0], &held[1], &held[2], &held[3], &held[4]); err != nil {
		t.Fatalf("counting the seeded rows: %v", err)
	}
	return held
}

// mail returns the id of the seeded mail stored under the reference ref.
func (s seededWorkspace) mail(t *testing.T, it *ingestTest, ref string) uuid.UUID {
	t.Helper()
	id, err := readMessageIDByProviderRef(t.Context(), it.pool, s.tenant, s.box.id, ref)
	if err != nil {
		t.Fatalf("reading the seeded mail %s: %v", ref, err)
	}
	return id
}

// linkedTo returns the known emails whose contacts the seeded mail stored under ref is linked to, sorted.
func (s seededWorkspace) linkedTo(t *testing.T, it *ingestTest, ref string) []string {
	t.Helper()
	rows, _ := it.pool.Query(t.Context(), `SELECT c.name FROM plugin_email.message_contacts l
		JOIN core.contacts c ON c.id = l.contact_id WHERE l.message_id = $1 ORDER BY c.name`, s.mail(t, it, ref))
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading the links of %s: %v", ref, err)
	}
	return names
}

// threadSubjects reads the subjects of the thread of the seeded mail stored under ref, in time order.
func (s seededWorkspace) threadSubjects(t *testing.T, it *ingestTest, ref string) string {
	t.Helper()
	var subjects string
	thread := storedRow(t, it.pool, s.mail(t, it, ref)).threadID
	if err := it.pool.QueryRow(t.Context(), seededThreadSubjects, thread).Scan(&subjects); err != nil {
		t.Fatalf("reading the thread of %s: %v", ref, err)
	}
	return subjects
}

// fileContent reads the bytes the store keeps under id in the seeded workspace.
func (s seededWorkspace) fileContent(t *testing.T, it *ingestTest, id uuid.UUID) []byte {
	t.Helper()
	file, err := it.store.Open(sdk.WithTenant(t.Context(), s.tenant), id)
	if err != nil {
		t.Fatalf("opening the file %v: %v", id, err)
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("reading the file %v: %v", id, err)
	}
	return content
}

func TestSeedStoresTheSampleMailboxWithItsThreadsMailsFilesAndLinks(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	seeded := it.seedWorkspace()

	if got, want := seeded.counts(t, it), [5]int{1, 3, 5, 2, 6}; got != want {
		t.Errorf("the seed stored %v mailboxes, threads, mails, attachments and links, want %v", got, want)
	}
	if seeded.box.address != "info@example.com" || !slices.Equal(seeded.box.aliases, []string{"sales@example.com"}) {
		t.Errorf("the sample mailbox is %q with %q, want info@example.com with sales@example.com",
			seeded.box.address, seeded.box.aliases)
	}
}

func TestSeedThreadsTheSampleMailOnTheSeededContacts(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	seeded := it.seedWorkspace()
	threads := map[string]string{
		"seed:lamp-quote":    "Quote for the new lamp | Re: Quote for the new lamp | Re: Quote for the new lamp",
		"seed:site-visit":    "Site visit on Friday",
		"seed:delivery-date": "Delivery date",
	}
	links := map[string][]string{
		"seed:lamp-quote":       {"maria.perez@example.com"},
		"seed:lamp-quote-reply": {"maria.perez@example.com"},
		"seed:lamp-photo":       {"maria.perez@example.com"},
		"seed:site-visit":       {"ada@example.com", "maria.perez@example.com"},
		"seed:delivery-date":    {"ada@example.com"},
	}

	gotThreads, gotLinks := map[string]string{}, map[string][]string{}
	for ref := range threads {
		gotThreads[ref] = seeded.threadSubjects(t, it, ref)
	}
	for ref := range links {
		gotLinks[ref] = seeded.linkedTo(t, it, ref)
	}

	if !maps.Equal(gotThreads, threads) {
		t.Errorf("the threads list %q, want %q in time order", gotThreads, threads)
	}
	if !reflect.DeepEqual(gotLinks, links) {
		t.Errorf("the mails are linked to %q, want %q", gotLinks, links)
	}
	if reply := storedRow(t, it.pool, seeded.mail(t, it, "seed:lamp-quote-reply")); reply.direction != outbound {
		t.Errorf("the sent reply is stored %s, want outbound", reply.direction)
	}
}

func TestSeedKeepsAnHTMLMailAPDFAndAnInlineImage(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	seeded := it.seedWorkspace()

	request := storedRow(t, it.pool, seeded.mail(t, it, "seed:lamp-quote"))
	quote := it.attachmentsOf(seeded.mail(t, it, "seed:lamp-quote-reply"))
	photo := it.attachmentsOf(seeded.mail(t, it, "seed:lamp-photo"))

	if !strings.Contains(request.safeHTML, "wicker lamp") || request.bodyText == "" {
		t.Errorf("the first mail keeps the HTML %q and the text %q, want an HTML mail", request.safeHTML, request.bodyText)
	}
	if len(quote) != 1 || quote[0].filename != "quote.pdf" || quote[0].contentType != "application/pdf" ||
		quote[0].inline || quote[0].state != fileStored {
		t.Fatalf("the sent reply keeps %+v, want quote.pdf kept as an attachment", quote)
	}
	if got := seeded.fileContent(t, it, quote[0].fileID); !bytes.Equal(got, quotePDF()) {
		t.Errorf("the kept quote holds %q, want the PDF the seed builds", got)
	}
	if len(photo) != 1 || photo[0].filename != "lamp.png" || photo[0].contentType != "image/png" ||
		!photo[0].inline || photo[0].contentID != "lamp-photo@example.com" || photo[0].state != fileStored {
		t.Fatalf("the photo mail keeps %+v, want lamp.png kept as an inline image", photo)
	}
	want, err := base64.StdEncoding.DecodeString(lampPNG)
	if err != nil {
		t.Fatalf("decoding the sample image: %v", err)
	}
	if got := seeded.fileContent(t, it, photo[0].fileID); !bytes.Equal(got, want) {
		t.Errorf("the kept image holds %q, want the one pixel PNG", got)
	}
}

func TestSeedDatesEachMailAtTheTimeTheMailboxReceivedIt(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	seeded := it.seedWorkspace()

	for _, ref := range []string{"seed:lamp-quote", "seed:lamp-quote-reply", "seed:lamp-photo", "seed:site-visit",
		"seed:delivery-date"} {
		row := storedRow(t, it.pool, seeded.mail(t, it, ref))
		if !row.sentAt.Equal(row.receivedAt) || !row.receivedAt.Before(ingestNow) {
			t.Errorf("the mail %s was sent %v and received %v, want both the same time before %v",
				ref, row.sentAt, row.receivedAt, ingestNow)
		}
	}
}

func TestSeedRunTwiceStoresNothingNewAndSavesNoFile(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	seeded := it.seedWorkspace()
	before, saved := seeded.counts(t, it), len(it.store.savedTypes())

	if err := it.ingester.seed(t.Context(), seeded.tenant); err != nil {
		t.Fatalf("seed() again error = %v, want nil", err)
	}

	if after := seeded.counts(t, it); after != before {
		t.Errorf("the second seed left %v rows, want %v as the first left them", after, before)
	}
	if again := len(it.store.savedTypes()); again != saved {
		t.Errorf("the second seed saved %d files, want none", again-saved)
	}
	if pending := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1",
		seeded.tenant); pending != 0 {
		t.Errorf("the seeds left %d files queued, want none", pending)
	}
}

func TestSeedPublishesNothing(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	it.seedWorkspace()

	if events := it.events(); len(events) != 0 {
		t.Errorf("the seed published %v, want no event", events)
	}
}

func TestSeedReportsAMailboxItCannotStore(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	err := it.ingester.seed(t.Context(), uuid.Must(uuid.NewV7()))

	if err == nil || !strings.Contains(err.Error(), "ensure mailbox") {
		t.Errorf("seed() of a workspace that does not exist error = %v, want the refused mailbox", err)
	}
}

func TestSeedReportsASampleMailItCannotStore(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.gate.err = errGateDown

	err := it.ingester.seed(t.Context(), sdk.DefaultTenantID)

	if !errors.Is(err, errGateDown) || !strings.Contains(err.Error(), "seed:lamp-quote") {
		t.Errorf("seed() error = %v, want the gate's failure naming the first sample mail", err)
	}
}

func TestQuotePDFPointsItsCrossReferenceAtEachObject(t *testing.T) {
	t.Parallel()

	pdf := quotePDF()

	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4\n")) || !bytes.HasSuffix(pdf, []byte("\n%%EOF\n")) {
		t.Fatalf("quotePDF() = %q, want a PDF 1.4 file that ends with its EOF mark", pdf)
	}
	text := string(pdf)
	start := strings.LastIndex(text, "startxref\n")
	table, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(text[start+len("startxref\n"):], "%%EOF\n")))
	if err != nil || !strings.HasPrefix(text[table:], "xref\n0 6\n0000000000 65535 f \n") {
		t.Fatalf("startxref names %d, %v, want the start of a cross reference table of six entries", table, err)
	}
	entries := crossReferenceEntry.FindAllStringSubmatch(text[table:], -1)
	if len(entries) != 5 {
		t.Fatalf("the cross reference table holds %d objects, want 5", len(entries))
	}
	for i, entry := range entries {
		offset, _ := strconv.Atoi(entry[1])
		if want := fmt.Sprintf("%d 0 obj\n", i+1); !strings.HasPrefix(text[offset:], want) {
			t.Errorf("entry %d points at %q, want %q", i+1, text[offset:offset+len(want)], want)
		}
	}
	stream := regexp.MustCompile(`/Length (\d+) >>\nstream\n`).FindStringSubmatchIndex(text)
	length, _ := strconv.Atoi(text[stream[2]:stream[3]])
	if !strings.HasPrefix(text[stream[1]+length:], "\nendstream") {
		t.Errorf("the content stream is not %d bytes long, want its Length to hold", length)
	}
}

func TestBase64LinesCutsTheEncodingAtTheMIMELineLength(t *testing.T) {
	t.Parallel()

	content := bytes.Repeat([]byte("lamp"), 50)

	lines := base64Lines(content)

	for _, line := range lines[:len(lines)-1] {
		if len(line) != 76 {
			t.Errorf("a line holds %d letters, want 76 on every line but the last", len(line))
		}
	}
	if last := lines[len(lines)-1]; len(last) == 0 || len(last) > 76 {
		t.Errorf("the last line holds %d letters, want between 1 and 76", len(last))
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.Join(lines, ""))
	if err != nil || !bytes.Equal(decoded, content) {
		t.Errorf("the lines decode to %q, %v, want the content", decoded, err)
	}
}

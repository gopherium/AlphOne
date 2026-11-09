// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

func TestIngestStoresASentCopyAsOutboundOnItsRecipientsAndBcc(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	sent := it.incoming(infoMail("Bcc: Ada Lovelace <ada@example.com>"), "file:sent")
	sent.SentCopy = true

	outcome := it.ingest(sent)

	if outcome != outcomeStored {
		t.Fatalf("ingest() = %q, want %q", outcome, outcomeStored)
	}
	id := it.mailTitled("Your quote")
	if row := storedRow(t, it.pool, id); row.direction != outbound {
		t.Errorf("the mail is %s, want outbound", row.direction)
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com", "ada@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez and Ada", links)
	}
}

func TestIngestMatchesNoBccOfAnInboundMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	outcome := it.ingest(it.incoming(strangerMail("Bcc: Ada Lovelace <ada@example.com>"), "file:offer"))

	if outcome != outcomeNoKnownContact {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeNoKnownContact)
	}
	if batches := it.matcher.calls(); !reflect.DeepEqual(batches, [][]string{{"stranger@example.com"}}) {
		t.Errorf("the matcher was asked %q, want the stranger alone", batches)
	}
}

func TestIngestMatchesTheAddressesInBatchesOfTheMatchMax(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingester.matchMax = 2

	it.ingest(it.incoming(lampMail("Cc: a@example.com, b@example.com, c@example.com, d@example.com"), "file:quote"))

	want := [][]string{{"maria.perez@example.com", "a@example.com"}, {"b@example.com", "c@example.com"},
		{"d@example.com"}}
	if batches := it.matcher.calls(); !reflect.DeepEqual(batches, want) {
		t.Errorf("the matcher was asked %q, want %q", batches, want)
	}
	if channels := it.matcher.channels; !slices.Equal(channels, []sdk.Channel{"email", "email", "email"}) {
		t.Errorf("the matcher was asked on %q, want the email channel every time", channels)
	}
}

func TestIngestStopsMatchingAtTheRecipientCap(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.caps.recipientsMax = 3 })

	it.ingest(it.incoming(rawMail("Cc: a@example.com, ada@example.com", "From: Maria Perez <maria.perez@example.com>",
		"To: info@example.com", "Subject: Your quote", "Content-Type: text/plain", "", lampText), "file:quote"))

	if batches := it.matcher.calls(); !reflect.DeepEqual(batches, [][]string{{"maria.perez@example.com",
		"a@example.com"}}) {
		t.Errorf("the matcher was asked %q, want the first three addresses less the mailbox", batches)
	}
	id := it.mailTitled("Your quote")
	if row := storedRow(t, it.pool, id); len(row.participants) != 3 {
		t.Errorf("the mail names %d participants, want 3", len(row.participants))
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez alone", links)
	}
}

func TestIngestCutsTheSubjectTheNamesAndTheParticipantsAtTheirCaps(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) {
		s.caps.subjectMaxBytes, s.caps.nameMaxBytes, s.caps.recipientsMax = 4, 5, 2
	})

	it.ingest(it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: Front Desk <info@example.com>", "Cc: ada@example.com",
		"Subject: =?UTF-8?Q?Caf=C3=A9_lamp?=", "Date: "+quoteDate, "Message-ID: <cafe@example.com>",
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="files"`, "",
		"--files", "Content-Type: text/plain", "", "The quote is attached.",
		"--files", "Content-Type: application/pdf", `Content-Disposition: attachment; filename="quotation.pdf"`, "",
		"%PDF-1.4", "--files--"), "file:cafe"))

	id := it.mailTitled("Caf")
	row := storedRow(t, it.pool, id)
	want := []participant{
		{Role: "from", Name: "Maria", Email: "maria.perez@example.com"},
		{Role: "to", Name: "Front", Email: "info@example.com"},
	}
	if !reflect.DeepEqual(row.participants, want) {
		t.Errorf("the participants are %+v, want %+v", row.participants, want)
	}
	if files := it.attachmentsOf(id); len(files) != 1 || files[0].filename != "quota" {
		t.Errorf("the attachments are %+v, want quotation.pdf cut to quota", files)
	}
	var subject string
	if err := it.pool.QueryRow(t.Context(), "SELECT subject FROM plugin_email.threads WHERE id = $1",
		row.threadID).Scan(&subject); err != nil || subject != "Caf" {
		t.Errorf("the thread keeps the subject %q, %v, want Caf", subject, err)
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez alone, Ada cut with the participants", links)
	}
}

func TestIngestKeepsTheContentIDACidSourceDecodesTo(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	it.ingest(it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: The wall",
		"MIME-Version: 1.0", `Content-Type: multipart/related; boundary="related"`, "",
		"--related", "Content-Type: text/html", "", `<p>The wall</p><img src="cid:a%2Bb@example.com">`,
		"--related", "Content-Type: image/png", "Content-ID: <a+b@example.com>", "Content-Transfer-Encoding: base64",
		"", "iVBORw0KGgo=", "--related--"), "file:wall"))

	id := it.mailTitled("The wall")
	row := storedRow(t, it.pool, id)
	if !strings.Contains(row.safeHTML, `src="cid:a%2Bb@example.com"`) {
		t.Errorf("the safe HTML is %q, want the cid source kept as sent", row.safeHTML)
	}
	files := it.attachmentsOf(id)
	decoded, err := url.PathUnescape("a%2Bb@example.com")
	if err != nil || len(files) != 1 || !files[0].inline || files[0].contentID != decoded {
		t.Errorf("the attachments are %+v, want one inline image whose content id is %q", files, decoded)
	}
}

func TestIngestKeepsNoSnippetForAnEncryptedMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	it.ingest(it.incoming(rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com",
		"Subject: Sealed", "Content-Type: text/plain", "", "-----BEGIN PGP MESSAGE-----", "hQEMA0123456789",
		"-----END PGP MESSAGE-----"), "file:sealed"))

	row := storedRow(t, it.pool, it.mailTitled("Sealed"))
	if !row.encrypted || row.snippet != "" || !strings.Contains(row.bodyText, "BEGIN PGP MESSAGE") {
		t.Errorf("the mail is encrypted %t with the snippet %q, want encrypted with no snippet and its text kept",
			row.encrypted, row.snippet)
	}
	if events := it.events(); len(events) != 1 || events[0].data["text"] != "" {
		t.Errorf("the events are %+v, want one with an empty text", events)
	}
}

func TestIngestCutsAContentIDAtTheNameCap(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	long := strings.Repeat("x", 200<<10)

	it.ingest(it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: Long id",
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="files"`, "",
		"--files", "Content-Type: text/plain", "", "The quote is attached.",
		"--files", "Content-Type: application/pdf", "Content-ID: <"+long+">",
		`Content-Disposition: attachment; filename="quote.pdf"`, "", "%PDF-1.4", "--files--"), "file:long-id"))

	files := it.attachmentsOf(it.mailTitled("Long id"))
	if len(files) != 1 {
		t.Fatalf("the mail keeps %d attachments, want quote.pdf alone", len(files))
	}
	if files[0].contentID != long[:defaultNameMaxBytes] {
		t.Errorf("the attachment keeps a content id of %d bytes, want it cut to %d", len(files[0].contentID),
			defaultNameMaxBytes)
	}
}

func TestIngestStoresEachAttachmentWithTheFileKeptForIt(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.files.attachmentMaxBytes = 16 })
	large := strings.Repeat("x", 40)

	it.ingest(it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: Three files",
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="outer"`, "",
		"--outer", `Content-Type: multipart/related; boundary="inner"`, "",
		"--inner", "Content-Type: text/html", "", `<p>The wall</p><img src="cid:wall@example.com">`,
		"--inner", "Content-Type: image/png", "Content-ID: <wall@example.com>", "Content-Transfer-Encoding: base64",
		"", "iVBORw0KGgo=", "--inner--",
		"--outer", "Content-Type: application/pdf", `Content-Disposition: attachment; filename="quote.pdf"`, "",
		"%PDF-1.4 quote",
		"--outer", "Content-Type: application/octet-stream", `Content-Disposition: attachment; filename="big.bin"`, "",
		large, "--outer--"), "file:three"))

	files := it.attachmentsOf(it.mailTitled("Three files"))
	if len(files) != 3 || files[0].fileID == uuid.Nil || files[1].fileID != uuid.Nil || files[2].fileID == uuid.Nil {
		t.Fatalf("the attachments are %+v, want a file for quote.pdf and the image and none for big.bin", files)
	}
	for i := range files {
		files[i].fileID = uuid.Nil
	}
	png := string([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	want := []attachmentRow{
		{position: 0, filename: "quote.pdf", contentType: "application/pdf", size: 14, sha256: sum("%PDF-1.4 quote"),
			state: fileStored},
		{position: 1, filename: "big.bin", contentType: octetStream, size: 40, sha256: sum(large), state: fileTooLarge},
		{position: 2, contentType: "image/png", size: 8, sha256: sum(png), inline: true, contentID: "wall@example.com",
			state: fileStored},
	}
	if !reflect.DeepEqual(files, want) {
		t.Errorf("the attachments are %+v, want %+v", files, want)
	}
}

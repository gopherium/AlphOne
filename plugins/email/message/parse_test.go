// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
)

// testLimits is wide enough for every fixture.
var testLimits = message.Limits{
	MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10,
	MaxHeaderBytes: 1 << 16, MaxBodyBytes: 1 << 20,
}

// deadline is how long a parse of a hostile message may take.
const deadline = 2 * time.Second

// outsideSender is the mailbox writing to the mailbox under test.
var outsideSender = message.Address{Name: "Maria Perez", Email: "maria.perez@example.com"}

// ownMailbox is the mailbox under test with its display name.
var ownMailbox = message.Address{Name: "Info", Email: "info@example.com"}

// bareOwnMailbox is the mailbox under test with no display name.
var bareOwnMailbox = message.Address{Email: "info@example.com"}

// ownAlias is the alias of the mailbox under test.
var ownAlias = message.Address{Name: "Sales", Email: "sales@example.com"}

// toOwnMailbox is a recipient list holding only the mailbox under test.
var toOwnMailbox = []message.Address{ownMailbox}

// fileSummary is the name and type TestParseFixtures checks of one attachment.
type fileSummary struct {
	name        string
	contentType string
}

// fixtureSummary is what TestParseFixtures checks of one parsed fixture.
type fixtureSummary struct {
	from        message.Address
	to          []message.Address
	cc          []message.Address
	bcc         []message.Address
	replyTo     []message.Address
	subject     string
	date        time.Time
	messageID   string
	inReplyTo   []string
	references  []string
	text        bool
	html        bool
	attachments []fileSummary
	inline      []string
}

// fixtureCases lists every fixture and the summary its parse answers.
var fixtureCases = []struct {
	file string
	want fixtureSummary
}{
	{"abuse-report.eml", fixtureSummary{
		from: message.Address{Name: "Abuse Desk", Email: "abuse@example.com"}, to: []message.Address{bareOwnMailbox},
		subject: "Abuse report for a message from 192.0.2.10", date: october(28, 7, 0),
		messageID: "abuse-report@example.com", text: true,
		attachments: []fileSummary{{"", "message/feedback-report"}, {"", "text/rfc822-headers"}},
	}},
	{"apple-mail-reply.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Re: Site visit", date: october(27, 8, 40),
		messageID: "apple-mail-reply@example.com", inReplyTo: []string{"visit-1@example.com"},
		references: []string{"visit-1@example.com"}, text: true,
	}},
	{"auto-reply.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Automatic reply: Quote", date: october(26, 10, 16),
		messageID: "auto-reply@example.com", inReplyTo: []string{"quote-1@example.com"},
		references: []string{"quote-1@example.com"}, text: true,
	}},
	{"calendar-invite.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Invitation: Site visit", date: october(29, 13, 0),
		messageID: "calendar-invite@example.com", text: true, attachments: []fileSummary{{"", "text/calendar"}},
	}},
	{"delivery-failure.eml", fixtureSummary{
		from: message.Address{Name: "Mail Delivery System", Email: "mailer-daemon@example.com"},
		to:   []message.Address{bareOwnMailbox}, subject: "Undelivered Mail Returned to Sender",
		date: october(26, 10, 17), messageID: "delivery-failure@example.com", text: true,
		attachments: []fileSummary{{"", "message/delivery-status"}, {"", "text/rfc822-headers"}},
	}},
	{"disposition-without-type.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Notes", date: october(29, 14, 0),
		messageID: "disposition-without-type@example.com", text: true, attachments: []fileSummary{{"", ""}},
	}},
	{"gmail-reply.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, cc: []message.Address{ownAlias}, subject: "Re: Quote",
		date: october(27, 9, 30), messageID: "gmail-reply@example.com", inReplyTo: []string{"quote-1@example.com"},
		references: []string{"quote-1@example.com"}, text: true, html: true,
	}},
	{"html-only.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Call me", date: october(28, 14, 0),
		messageID: "html-only@example.com", text: true, html: true,
	}},
	{"inline-image.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Photo of the wall", date: october(27, 11, 0),
		messageID: "inline-image@example.com", text: true, html: true, inline: []string{"img1@example.com"},
	}},
	{"latin1-text.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "New façade at £120", date: october(28, 13, 0),
		messageID: "latin1-text@example.com", text: true,
	}},
	{"list-mail.eml", fixtureSummary{
		from: message.Address{Name: "Example News", Email: "news@example.com"}, to: []message.Address{bareOwnMailbox},
		replyTo: []message.Address{{Name: "News Desk", Email: "desk@example.com"}}, subject: "October news",
		date: october(22, 6, 0), messageID: "list-mail@example.com", text: true,
	}},
	{"malformed-references.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Re: Quote", date: october(29, 9, 0),
		messageID: "malformed-references@example.com", inReplyTo: []string{"quote-3@example.com"},
		references: []string{"quote-1@example.com", "quote-2@example.com", "quote-3@example.com"}, text: true,
	}},
	{"mixed-case-sender.eml", fixtureSummary{
		from: outsideSender, to: []message.Address{bareOwnMailbox}, subject: "Quote", date: october(29, 8, 0),
		messageID: "mixed-case-sender@example.com", text: true,
	}},
	{"outlook-reply.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "RE: Site visit", date: october(27, 8, 5),
		messageID: "outlook-reply@example.com", inReplyTo: []string{"visit-1@example.com"},
		references: []string{"visit-1@example.com"}, text: true,
	}},
	{"pdf-attachment.eml", fixtureSummary{
		from: outsideSender, to: []message.Address{ownAlias}, subject: "Signed quote", date: october(28, 12, 0),
		messageID: "pdf-attachment@example.com", text: true,
		attachments: []fileSummary{{"quote.pdf", "application/pdf"}},
	}},
	{"pgp-encrypted.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Encrypted plans", date: october(29, 12, 30),
		messageID: "pgp-encrypted@example.com",
		attachments: []fileSummary{
			{"", "application/pgp-encrypted"}, {"encrypted.asc", "application/octet-stream"},
		},
	}},
	{"pgp-signed.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Signed confirmation", date: october(29, 12, 0),
		messageID: "pgp-signed@example.com", text: true,
		attachments: []fileSummary{{"signature.asc", "application/pgp-signature"}},
	}},
	{"read-receipt.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Read: Quote", date: october(27, 9, 10),
		messageID: "read-receipt@example.com", text: true,
		attachments: []fileSummary{{"", "message/disposition-notification"}},
	}},
	{"smime-encrypted.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Encrypted order", date: october(29, 11, 30),
		messageID: "smime-encrypted@example.com", attachments: []fileSummary{{"smime.p7m", "application/pkcs7-mime"}},
	}},
	{"smime-signed.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Signed order", date: october(29, 11, 0),
		messageID: "smime-signed@example.com", text: true,
		attachments: []fileSummary{{"smime.p7s", "application/pkcs7-signature"}},
	}},
	{"thunderbird-reply.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Re: Site visit", date: october(27, 10, 2),
		messageID: "thunderbird-reply@example.com", inReplyTo: []string{"visit-1@example.com"},
		references: []string{"visit-1@example.com"}, text: true,
	}},
	{"winmail-dat.eml", fixtureSummary{
		from: outsideSender, to: toOwnMailbox, subject: "Measurements", date: october(29, 10, 0),
		messageID: "winmail-dat@example.com", text: true,
		attachments: []fileSummary{{"winmail.dat", "application/ms-tnef"}},
	}},
}

// cleanTextCases lists raw messages, the string to read from each and the clean text Parse answers there.
var cleanTextCases = []struct {
	name string
	raw  string
	read func(*message.Message) string
	want string
}{
	{"a raw NUL in the subject", "Subject: Qu\x00ote\r\n\r\nText.\r\n", subjectOf, "Quote"},
	{"an encoded NUL in the subject", "Subject: =?utf-8?q?Qu=00ote?=\r\n\r\nText.\r\n", subjectOf, "Quote"},
	{"a raw broken byte in the subject", "Subject: Qu\xffote\r\n\r\nText.\r\n", subjectOf, "Qu" + replacement + "ote"},
	{
		"an encoded broken byte in the subject", "Subject: =?utf-8?q?Qu=FFote?=\r\n\r\nText.\r\n", subjectOf,
		"Qu" + replacement + "ote",
	},
	{"a NUL in the text", "Content-Type: text/plain\r\n\r\nTe\x00xt.", textOf, "Text."},
	{
		"an encoded NUL in the text",
		"Content-Type: text/plain\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nTe=00xt.", textOf, "Text.",
	},
	{
		"a broken byte in the text", "Content-Type: text/plain; charset=utf-8\r\n\r\nTe\xffxt.", textOf,
		"Te" + replacement + "xt.",
	},
	{"a NUL in the HTML", "Content-Type: text/html\r\n\r\n<p>Te\x00xt.</p>", htmlOf, "<p>Text.</p>"},
	{
		"a broken byte in the HTML", "Content-Type: text/html; charset=utf-8\r\n\r\n<p>Te\xffxt.</p>", htmlOf,
		"<p>Te" + replacement + "xt.</p>",
	},
	{
		"a raw NUL in a sender name", "From: \"Maria\x00 Perez\" <maria.perez@example.com>\r\n\r\nText.\r\n",
		fromNameOf, "Maria Perez",
	},
	{
		"an encoded NUL in a sender name", "From: =?utf-8?q?Maria=00_Perez?= <maria.perez@example.com>\r\n\r\nText.\r\n",
		fromNameOf, "Maria Perez",
	},
	{
		"a broken byte in a recipient name", "To: =?utf-8?q?Maria=FF?= <maria.perez@example.com>\r\n\r\nText.\r\n",
		toNameOf, "Maria" + replacement,
	},
	{
		"an encoded NUL in a file name", fileWithField("Content-Disposition: attachment; filename*=utf-8''no%00te.pdf"),
		fileNameOf, "note.pdf",
	},
	{
		"a broken byte in a file name", fileWithField("Content-Disposition: attachment; filename*=utf-8''no%FFte.pdf"),
		fileNameOf, "no" + replacement + "te.pdf",
	},
	{"a NUL in a content id", fileWithField("Content-ID: <no\x00te@example.com>"), contentIDOf, "note@example.com"},
	{
		"an escaped NUL in a content id", fileWithField("Content-ID: <no%00te@example.com>"), contentIDOf,
		"note@example.com",
	},
	{
		"a broken byte in a content id", fileWithField("Content-ID: <no\xffte@example.com>"), contentIDOf,
		"no" + replacement + "te@example.com",
	},
	{
		"a broken byte beside a broken escape in a content id", fileWithField("Content-ID: <no%zz\xffte@example.com>"),
		contentIDOf, "no%zz" + replacement + "te@example.com",
	},
}

func TestParseFixtures(t *testing.T) {
	t.Parallel()

	var listed []string
	for _, c := range fixtureCases {
		listed = append(listed, c.file)
		t.Run(c.file, func(t *testing.T) {
			t.Parallel()

			if got := summarize(parseFixture(t, c.file)); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Parse(%s) =\n%+v\nwant\n%+v", c.file, got, c.want)
			}
		})
	}
	if files := fixtureNames(t); !slices.Equal(listed, files) {
		t.Errorf("fixture cases list %q, want every fixture in testdata %q", listed, files)
	}
}

func TestParseKeepsTheRawBytes(t *testing.T) {
	t.Parallel()

	for _, name := range fixtureNames(t) {
		raw := readFixture(t, name)

		m, err := message.Parse(bytes.NewReader(raw), testLimits)

		if err != nil || !bytes.Equal(m.Raw, raw) {
			t.Errorf("Parse(%s) answered other raw bytes than it read, error %v", name, err)
		}
	}
}

func TestParseReadsTheThreadIndex(t *testing.T) {
	t.Parallel()

	index := parseFixture(t, "outlook-reply.eml").ThreadIndex

	wantGUID := [16]byte{0xa1, 0xb2, 0xc3, 0xd4, 0xe5, 0xf6, 0x07, 0x18, 0x29, 0x3a, 0x4b, 0x5c, 0x6d, 0x7e, 0x8f, 0x90}
	if index == nil {
		t.Fatal("Parse(outlook-reply.eml).ThreadIndex = nil, want a thread index")
	}
	if index.GUID != wantGUID {
		t.Errorf("Parse(outlook-reply.eml).ThreadIndex.GUID = %x, want %x", index.GUID, wantGUID)
	}
	if index.Replies != 1 {
		t.Errorf("Parse(outlook-reply.eml).ThreadIndex.Replies = %d, want 1", index.Replies)
	}
}

func TestParseRefusesAMalformedMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
	}{
		{"a multipart message with no boundary", "Content-Type: multipart/mixed\r\n\r\nThe boundary is missing.\r\n"},
		{"a top type that cannot be read", "Content-Type: a b\r\n\r\nThe type is broken.\r\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if _, err := message.Parse(strings.NewReader(c.raw), testLimits); !errors.Is(err, message.ErrMalformed) {
				t.Errorf("Parse() of %s error = %v, want ErrMalformed", c.name, err)
			}
		})
	}
}

func TestParseBytesAnswersWhatParseAnswers(t *testing.T) {
	t.Parallel()

	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			raw := readFixture(t, name)

			m, err := message.ParseBytes(raw, testLimits)

			if err != nil {
				t.Fatalf("ParseBytes(%s) error = %v, want nil", name, err)
			}
			if got, want := summarize(m), summarize(parseFixture(t, name)); !reflect.DeepEqual(got, want) {
				t.Errorf("ParseBytes(%s) =\n%+v\nwant\n%+v", name, got, want)
			}
			if &m.Raw[0] != &raw[0] {
				t.Errorf("ParseBytes(%s).Raw is a copy, want the bytes it was given", name)
			}
		})
	}
}

func TestParseBytesChecksItsLimits(t *testing.T) {
	t.Parallel()

	raw := readFixture(t, "gmail-reply.eml")
	shortCap := testLimits
	shortCap.MaxBytes = int64(len(raw)) - 1

	if _, err := message.ParseBytes(raw, message.Limits{}); !errors.Is(err, message.ErrLimitsUnset) {
		t.Errorf("ParseBytes() with no limits error = %v, want ErrLimitsUnset", err)
	}
	if _, err := message.ParseBytes(raw, shortCap); !errors.Is(err, message.ErrTooLarge) {
		t.Errorf("ParseBytes() over the byte cap error = %v, want ErrTooLarge", err)
	}
}

func TestParseReadsBcc(t *testing.T) {
	t.Parallel()

	raw := "Bcc: Maria Perez <maria.perez@example.com>, info@example.com\r\nContent-Type: text/plain\r\n\r\nText.\r\n"

	m, err := message.Parse(strings.NewReader(raw), testLimits)

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if want := []message.Address{outsideSender, bareOwnMailbox}; !slices.Equal(m.Bcc, want) {
		t.Errorf("Parse().Bcc = %+v, want %+v", m.Bcc, want)
	}
}

func TestParseKeepsTheGoodAddressesOfABrokenField(t *testing.T) {
	t.Parallel()

	withRawByte := message.Address{Name: "Maria Perez " + replacement + " Lamps", Email: "maria.perez@example.com"}
	raw := "From: Maria Perez \xa9 Lamps <maria.perez@example.com>\r\n" +
		"To: Maria Perez \xa9 Lamps <maria.perez@example.com>, info@example.com\r\n" +
		"Cc: sales@example.com, ??? junk, info@example.com\r\nContent-Type: text/plain\r\n\r\nText.\r\n"

	m, err := message.Parse(strings.NewReader(raw), testLimits)

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if m.From != withRawByte {
		t.Errorf("Parse().From = %+v, want %+v", m.From, withRawByte)
	}
	if want := []message.Address{withRawByte, bareOwnMailbox}; !slices.Equal(m.To, want) {
		t.Errorf("Parse().To = %+v, want %+v", m.To, want)
	}
	if want := []message.Address{{Email: "sales@example.com"}, bareOwnMailbox}; !slices.Equal(m.Cc, want) {
		t.Errorf("Parse().Cc = %+v, want %+v", m.Cc, want)
	}
}

func TestParseAnswersCleanText(t *testing.T) {
	t.Parallel()

	for _, c := range cleanTextCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			m, err := message.Parse(strings.NewReader(c.raw), testLimits)

			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if got := c.read(m); got != c.want {
				t.Errorf("Parse() of %q read %q, want %q", c.raw, got, c.want)
			}
			if !bytes.Equal(m.Raw, []byte(c.raw)) {
				t.Errorf("Parse().Raw = %q, want the bytes as read", m.Raw)
			}
		})
	}
}

func TestParseLeavesOutAnEncodedNULInAnEmail(t *testing.T) {
	t.Parallel()

	raw := "To: =?utf-8?q?a=00b?=@example.com, info@example.com\r\n\r\nText.\r\n"

	m, err := message.Parse(strings.NewReader(raw), testLimits)

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if want := []message.Address{bareOwnMailbox}; !slices.Equal(m.To, want) {
		t.Errorf("Parse().To = %+v, want %+v", m.To, want)
	}
}

func TestParseSurfacesAReadError(t *testing.T) {
	t.Parallel()

	raw := readFixture(t, "gmail-reply.eml")
	errBroken := errors.New("the connection broke")
	broken := io.MultiReader(bytes.NewReader(raw[:len(raw)/2]), iotest.ErrReader(errBroken))

	m, err := message.Parse(broken, testLimits)

	if !errors.Is(err, errBroken) || m != nil {
		t.Errorf("Parse() of a reader failing halfway = %+v, %v, want nil and the read error", m, err)
	}
}

func TestParseFillsTextFromHTMLOnlyMail(t *testing.T) {
	t.Parallel()

	m := parseFixture(t, "html-only.eml")

	if want := "Please call me about the *quote*.\n\nMaria Perez\n184467235"; m.Text != want {
		t.Errorf("Parse(html-only.eml).Text = %q, want %q", m.Text, want)
	}
}

func TestParseDecodesLatin1(t *testing.T) {
	t.Parallel()

	m := parseFixture(t, "latin1-text.eml")

	if want := "The new façade costs £120.\r\n"; !strings.HasPrefix(m.Text, want) {
		t.Errorf("Parse(latin1-text.eml).Text = %q, want it to start %q", m.Text, want)
	}
	if want := "New façade at £120"; m.Subject != want {
		t.Errorf("Parse(latin1-text.eml).Subject = %q, want %q", m.Subject, want)
	}
}

func TestParseLeavesMissingHeadersEmpty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
	}{
		{"no header but the type", "Content-Type: text/plain\r\n\r\nNo other header.\r\n"},
		{
			"unreadable headers",
			"From: <>\r\nTo: <>\r\nDate: not a date\r\nMessage-ID: <>\r\nThread-Index: not base64\r\n" +
				"Content-Type: text/plain\r\n\r\nEvery header is unreadable.\r\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			m, err := message.Parse(strings.NewReader(c.raw), testLimits)

			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if got := headersOf(m); !reflect.DeepEqual(got, headerSummary{}) {
				t.Errorf("Parse() headers = %+v, want no sender, recipient, date, id or thread index", got)
			}
		})
	}
}

func TestFixturesKeepCRLFLineEnds(t *testing.T) {
	t.Parallel()

	for _, name := range fixtureNames(t) {
		raw := readFixture(t, name)

		if bytes.Count(raw, []byte("\n")) != bytes.Count(raw, []byte("\r\n")) || !bytes.HasSuffix(raw, []byte("\r\n")) {
			t.Errorf("%s holds a line end other than CRLF", name)
		}
	}
}

// october answers the UTC time on the given day of October 2026.
func october(day, hour, minute int) time.Time {
	return time.Date(2026, time.October, day, hour, minute, 0, 0, time.UTC)
}

// fixtureNames lists the fixture files in testdata, sorted.
func fixtureNames(t testing.TB) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "*.eml"))
	if err != nil {
		t.Fatalf("filepath.Glob() error = %v", err)
	}
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, filepath.Base(path))
	}
	slices.Sort(names)
	return names
}

// readFixture answers the bytes of the fixture file name.
func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", name, err)
	}
	return raw
}

// parseFixture answers the fixture file name parsed under testLimits.
func parseFixture(t *testing.T, name string) *message.Message {
	t.Helper()
	m, err := message.Parse(bytes.NewReader(readFixture(t, name)), testLimits)
	if err != nil {
		t.Fatalf("Parse(%s) error = %v, want nil", name, err)
	}
	return m
}

// summarize answers what TestParseFixtures checks of m.
func summarize(m *message.Message) fixtureSummary {
	s := fixtureSummary{
		from: m.From, to: m.To, cc: m.Cc, bcc: m.Bcc, replyTo: m.ReplyTo, subject: m.Subject, date: m.Date.UTC(),
		messageID: m.MessageID, inReplyTo: m.InReplyTo, references: m.References, text: m.Text != "", html: m.HTML != "",
	}
	for _, a := range m.Attachments {
		s.attachments = append(s.attachments, fileSummary{a.Name, a.ContentType})
	}
	for _, a := range m.InlineImages {
		s.inline = append(s.inline, a.ContentID)
	}
	return s
}

// headerSummary is what the tests of missing headers check of a message.
type headerSummary struct {
	from        message.Address
	to          []message.Address
	dated       bool
	messageID   string
	threadIndex *message.ThreadIndex
}

// headersOf answers the header summary of m.
func headersOf(m *message.Message) headerSummary {
	return headerSummary{m.From, m.To, !m.Date.IsZero(), m.MessageID, m.ThreadIndex}
}

// subjectOf answers the subject of m.
func subjectOf(m *message.Message) string {
	return m.Subject
}

// textOf answers the text body of m.
func textOf(m *message.Message) string {
	return m.Text
}

// htmlOf answers the HTML body of m.
func htmlOf(m *message.Message) string {
	return m.HTML
}

// fromNameOf answers the name of the sender of m.
func fromNameOf(m *message.Message) string {
	return m.From.Name
}

// toNameOf answers the name of the first recipient of m.
func toNameOf(m *message.Message) string {
	return m.To[0].Name
}

// fileNameOf answers the name of the first attachment of m.
func fileNameOf(m *message.Message) string {
	return m.Attachments[0].Name
}

// contentIDOf answers the content id of the first attachment of m.
func contentIDOf(m *message.Message) string {
	return m.Attachments[0].ContentID
}

// fileWithField answers a raw message holding a text part and a PDF part with the header field given.
func fileWithField(field string) string {
	return fileMessage("Content-Type: text/plain\r\n\r\nText.", "Content-Type: application/pdf\r\n"+field)
}

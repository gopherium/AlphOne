// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/quotedprintable"
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
var testLimits = message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}

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

func TestParseKeepsTheContentOfEachFile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file  string
		files func(*message.Message) []message.Attachment
		want  string
	}{
		{"pdf-attachment.eml", attachmentsOf, "%PDF-1.4\n"},
		{"inline-image.eml", inlineImagesOf, "\x89PNG\r\n\x1a\n"},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			t.Parallel()

			files := c.files(parseFixture(t, c.file))

			if len(files) != 1 || !bytes.HasPrefix(files[0].Content, []byte(c.want)) {
				t.Errorf("Parse(%s) files = %+v, want one starting %q", c.file, files, c.want)
			}
		})
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

func TestParseRefusesAMessageOverTheByteCap(t *testing.T) {
	t.Parallel()

	raw := manyParts(3)

	limits := message.Limits{MaxBytes: int64(len(raw)) - 1, MaxParts: 1, MaxContainers: 1, MaxFieldBytes: 1 << 10}

	_, err := message.Parse(bytes.NewReader(raw), limits)

	if !errors.Is(err, message.ErrTooLarge) {
		t.Errorf("Parse() of %d bytes under a cap of %d error = %v, want ErrTooLarge", len(raw), len(raw)-1, err)
	}
}

func TestParseTakesAMessageAtTheByteCap(t *testing.T) {
	t.Parallel()

	for _, maxBytes := range []int64{int64(len(manyParts(3))), math.MaxInt64} {
		t.Run(fmt.Sprint(maxBytes), func(t *testing.T) {
			t.Parallel()

			limits := message.Limits{MaxBytes: maxBytes, MaxParts: 3, MaxContainers: 1, MaxFieldBytes: 1 << 10}

			m, err := message.Parse(bytes.NewReader(manyParts(3)), limits)

			if err != nil || len(m.Attachments) != 3 {
				t.Errorf("Parse() under a cap of %d error = %v, want three attachments", maxBytes, err)
			}
		})
	}
}

func TestParseRefusesUnsetLimits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		limits message.Limits
	}{
		{"a zero byte cap", message.Limits{MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}},
		{"a negative byte cap", message.Limits{MaxBytes: -1, MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}},
		{"a zero part limit", message.Limits{MaxBytes: 1 << 20, MaxContainers: 10, MaxFieldBytes: 1 << 10}},
		{"a negative part limit", message.Limits{MaxBytes: 1 << 20, MaxParts: -1, MaxContainers: 10, MaxFieldBytes: 1 << 10}},
		{"a zero container limit", message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxFieldBytes: 1 << 10}},
		{
			"a negative container limit",
			message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: -1, MaxFieldBytes: 1 << 10},
		},
		{"a zero field cap", message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: 10}},
		{"a negative field cap", message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: 10, MaxFieldBytes: -1}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			unread := iotest.ErrReader(errors.New("the reader was read"))

			if _, err := message.Parse(unread, c.limits); !errors.Is(err, message.ErrLimitsUnset) {
				t.Errorf("Parse() with limits %+v error = %v, want ErrLimitsUnset", c.limits, err)
			}
		})
	}
}

func TestParseRefusesTooManyParts(t *testing.T) {
	t.Parallel()

	raw := manyParts(4)

	limits := message.Limits{MaxBytes: int64(len(raw)), MaxParts: 3, MaxContainers: 1, MaxFieldBytes: 1 << 10}

	_, err := message.Parse(bytes.NewReader(raw), limits)

	if !errors.Is(err, message.ErrTooManyParts) {
		t.Errorf("Parse() of four parts under a limit of three error = %v, want ErrTooManyParts", err)
	}
}

func TestParseRefusesDeepNestingQuickly(t *testing.T) {
	raw := nestedParts(1000)
	limits := message.Limits{MaxBytes: int64(len(raw)), MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}
	done := make(chan error, 1)
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	go func() {
		_, err := message.Parse(bytes.NewReader(raw), limits)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, message.ErrTooManyParts) {
			t.Errorf("Parse() of 1000 nested parts under a container limit of 10 error = %v, want ErrTooManyParts", err)
		}
	case <-timer.C:
		t.Fatalf("Parse() of 1000 nested parts under a container limit of 10 took over %v", deadline)
	}
}

func TestParseRefusesUnclosedNestingQuickly(t *testing.T) {
	raw := unclosedParts(50)
	limits := message.Limits{MaxBytes: int64(len(raw)), MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}
	done := make(chan error, 1)
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	go func() {
		_, err := message.Parse(bytes.NewReader(raw), limits)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, message.ErrTooManyParts) {
			t.Errorf("Parse() of 50 unclosed nested parts under a container limit of 10 error = %v, want ErrTooManyParts", err)
		}
	case <-timer.C:
		t.Fatalf("Parse() of 50 unclosed nested parts under a container limit of 10 took over %v", deadline)
	}
}

func TestParseCountsEveryContainer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field string
		want  error
	}{
		{"a field on one line", "Content-Type: multipart/mixed; boundary=\"inner\"", message.ErrTooManyParts},
		{"a field folded over two lines", "Content-Type: multipart/mixed;\r\n boundary=\"inner\"", message.ErrTooManyParts},
		{
			"a field continued on a line with no indent",
			"Content-Type: multipart/mixed;\r\nboundary=\"inner\"", message.ErrTooManyParts,
		},
		{
			"a field continued past a line the reader skips",
			"Content-Type: multipart/mixed;\r\n:skipped\r\n boundary=\"inner\"", message.ErrTooManyParts,
		},
		{"a lowercase field name", "content-type: multipart/mixed; boundary=\"inner\"", message.ErrTooManyParts},
		{"a blank before the colon", "Content-Type : multipart/mixed; boundary=\"inner\"", message.ErrTooManyParts},
		{
			"a parameter name in an encoded word",
			"Content-Type: multipart/mixed; =?us-ascii?b?Ym91bmRhcnk=?==inner", message.ErrTooManyParts,
		},
		{
			"a boundary split into sections",
			"Content-Type: multipart/mixed; boundary*0=\"in\"; boundary*1=\"ner\"", message.ErrTooManyParts,
		},
		{"a field with no boundary", "Content-Type: text/plain", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			raw := "Content-Type: multipart/mixed; boundary=\"outer\"\r\n\r\n--outer\r\n" + c.field +
				"\r\n\r\n--inner\r\nContent-Type: text/plain\r\n\r\nText.\r\n--inner--\r\n--outer--\r\n"
			limits := message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: 1, MaxFieldBytes: 1 << 10}

			if _, err := message.Parse(strings.NewReader(raw), limits); !errors.Is(err, c.want) {
				t.Errorf("Parse() of a second container in %q under a limit of one error = %v, want %v", c.field, err, c.want)
			}
		})
	}
}

func TestParseTakesAMessageAtTheContainerLimit(t *testing.T) {
	t.Parallel()

	raw := nestedParts(2)

	for _, c := range []struct {
		maxContainers int
		want          error
	}{{3, nil}, {2, message.ErrTooManyParts}} {
		t.Run(fmt.Sprint(c.maxContainers), func(t *testing.T) {
			t.Parallel()

			limits := message.Limits{
				MaxBytes: int64(len(raw)), MaxParts: 100, MaxContainers: c.maxContainers, MaxFieldBytes: 1 << 10,
			}

			if _, err := message.Parse(bytes.NewReader(raw), limits); !errors.Is(err, c.want) {
				t.Errorf("Parse() of three containers under a limit of %d error = %v, want %v", c.maxContainers, err, c.want)
			}
		})
	}
}

func TestParseRefusesALongMediaFieldQuickly(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"a type field in the top part", "Content-Type: text/plain; " + manyParameters(80000) + "\r\n\r\nText.\r\n"},
		{
			"a disposition field in a nested part",
			"Content-Type: multipart/mixed; boundary=\"parts\"\r\n\r\n--parts\r\nContent-Type: text/plain\r\n" +
				"Content-Disposition: attachment; " + manyParameters(80000) + "\r\n\r\nText.\r\n--parts--\r\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			limits := message.Limits{MaxBytes: int64(len(c.raw)), MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}
			done := make(chan error, 1)
			timer := time.NewTimer(deadline)
			defer timer.Stop()

			go func() {
				_, err := message.Parse(strings.NewReader(c.raw), limits)
				done <- err
			}()

			select {
			case err := <-done:
				if !errors.Is(err, message.ErrTooLarge) {
					t.Errorf("Parse() of %d bytes with %s error = %v, want ErrTooLarge", len(c.raw), c.name, err)
				}
			case <-timer.C:
				t.Fatalf("Parse() of %d bytes with %s took over %v", len(c.raw), c.name, deadline)
			}
		})
	}
}

func TestParseTakesAMediaFieldAtTheFieldCap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field string
		value string
	}{
		{"a type field", "Content-Type: text/plain; charset=us-ascii", "text/plain; charset=us-ascii"},
		{"a folded type field", "Content-Type: text/plain;\r\n charset=us-ascii", "text/plain; charset=us-ascii"},
		{"a disposition field", "Content-Disposition: inline; filename=\"note.txt\"", "inline; filename=\"note.txt\""},
	}

	for _, c := range cases {
		for _, limit := range []struct {
			maxFieldBytes int
			want          error
		}{{len(c.value), nil}, {len(c.value) - 1, message.ErrTooLarge}} {
			t.Run(fmt.Sprint(c.name, " ", limit.maxFieldBytes), func(t *testing.T) {
				t.Parallel()

				raw := c.field + "\r\n\r\nText.\r\n"
				limits := message.Limits{
					MaxBytes: int64(len(raw)), MaxParts: 100, MaxContainers: 10, MaxFieldBytes: limit.maxFieldBytes,
				}

				if _, err := message.Parse(strings.NewReader(raw), limits); !errors.Is(err, limit.want) {
					t.Errorf("Parse() of %s under a field cap of %d error = %v, want %v",
						c.name, limit.maxFieldBytes, err, limit.want)
				}
			})
		}
	}
}

func TestParseSortsInlineImagesFromAttachments(t *testing.T) {
	t.Parallel()

	htmlShowingTheImage := "Content-Type: text/html\r\n\r\n<p><img src=\"cid:img1@example.com\"></p>"
	cases := []struct {
		name        string
		body        string
		file        string
		attachments []string
		inline      []string
	}{
		{
			"a PDF with a content id and an inline disposition",
			"Content-Type: text/plain\r\n\r\nThe quote is attached.",
			"Content-Type: application/pdf; name=\"quote.pdf\"\r\n" +
				"Content-Disposition: inline; filename=\"quote.pdf\"\r\nContent-ID: <pdf1@example.com>",
			[]string{"quote.pdf"}, nil,
		},
		{
			"a PDF with a content id and no disposition",
			"Content-Type: text/plain\r\n\r\nThe quote is attached.",
			"Content-Type: application/pdf; name=\"quote.pdf\"\r\nContent-ID: <pdf1@example.com>",
			[]string{"quote.pdf"}, nil,
		},
		{
			"an image the HTML never shows",
			"Content-Type: text/html\r\n\r\n<p>See the wall.</p>",
			"Content-Type: image/png; name=\"wall.png\"\r\nContent-ID: <img1@example.com>",
			[]string{"wall.png"}, nil,
		},
		{
			"an image with no content id",
			htmlShowingTheImage,
			"Content-Type: image/png; name=\"wall.png\"\r\nContent-Disposition: inline; filename=\"wall.png\"",
			[]string{"wall.png"}, nil,
		},
		{
			"an image the HTML shows through a cid link",
			htmlShowingTheImage,
			"Content-Type: image/png; name=\"wall.png\"\r\nContent-ID: <img1@example.com>",
			nil, []string{"wall.png"},
		},
		{
			"an image the HTML shows, sent as an attachment",
			htmlShowingTheImage,
			"Content-Type: image/png; name=\"wall.png\"\r\n" +
				"Content-Disposition: attachment; filename=\"wall.png\"\r\nContent-ID: <img1@example.com>",
			nil, []string{"wall.png"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			m, err := message.Parse(strings.NewReader(fileMessage(c.body, c.file)), testLimits)

			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if got := fileNames(m.Attachments); !slices.Equal(got, c.attachments) {
				t.Errorf("Parse().Attachments = %q, want %q", got, c.attachments)
			}
			if got := fileNames(m.InlineImages); !slices.Equal(got, c.inline) {
				t.Errorf("Parse().InlineImages = %q, want %q", got, c.inline)
			}
		})
	}
}

func TestParseRefusesAMalformedMessage(t *testing.T) {
	t.Parallel()

	raw := "Content-Type: multipart/mixed\r\n\r\nThe boundary is missing.\r\n"

	if _, err := message.Parse(strings.NewReader(raw), testLimits); !errors.Is(err, message.ErrMalformed) {
		t.Errorf("Parse() of a multipart message with no boundary error = %v, want ErrMalformed", err)
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
			got := headerSummary{m.From, m.To, !m.Date.IsZero(), m.MessageID, m.ThreadIndex}
			if !reflect.DeepEqual(got, headerSummary{}) {
				t.Errorf("Parse() headers = %+v, want no sender, recipient, date, id or thread index", got)
			}
		})
	}
}

func TestParseKeepsEveryByteOfALongQuotedPrintableAttachment(t *testing.T) {
	t.Parallel()

	payload := make([]byte, 5000)
	for i := range payload {
		payload[i] = byte(i)
	}
	var encoded bytes.Buffer
	writer := quotedprintable.NewWriter(&encoded)
	writer.Binary = true
	_, _ = writer.Write(payload)
	_ = writer.Close()
	raw := "Content-Type: multipart/mixed; boundary=\"long\"\r\n\r\n--long\r\n" +
		"Content-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=\"long.bin\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" + encoded.String() + "\r\n--long--\r\n"

	m, err := message.Parse(strings.NewReader(raw), testLimits)

	if err != nil {
		t.Fatalf("Parse() of a 5000 byte quoted-printable attachment error = %v, want nil", err)
	}
	if len(m.Attachments) != 1 {
		t.Fatalf("Parse() of a 5000 byte quoted-printable attachment gave %d attachments, want 1", len(m.Attachments))
	}
	if !bytes.Equal(m.Attachments[0].Content, payload) {
		t.Errorf("Parse() of a 5000 byte quoted-printable attachment lost bytes")
	}
}

func TestParsePinsAPartWithADispositionAndNoType(t *testing.T) {
	t.Parallel()

	attachments := parseFixture(t, "disposition-without-type.eml").Attachments

	want := message.Attachment{Content: []byte("Bring the colour samples on Friday.\r\n")}
	if len(attachments) != 1 || !reflect.DeepEqual(attachments[0], want) {
		t.Errorf("Parse(disposition-without-type.eml).Attachments = %+v, want only %+v", attachments, want)
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
func fixtureNames(t *testing.T) []string {
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
func readFixture(t *testing.T, name string) []byte {
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
		from: m.From, to: m.To, cc: m.Cc, replyTo: m.ReplyTo, subject: m.Subject, date: m.Date.UTC(),
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

// attachmentsOf answers the attachments of m.
func attachmentsOf(m *message.Message) []message.Attachment {
	return m.Attachments
}

// inlineImagesOf answers the inline images of m.
func inlineImagesOf(m *message.Message) []message.Attachment {
	return m.InlineImages
}

// manyParts answers a raw message holding count small attachments.
func manyParts(count int) []byte {
	var b strings.Builder
	b.WriteString("Content-Type: multipart/mixed; boundary=\"parts\"\r\n\r\n")
	for i := range count {
		fmt.Fprintf(&b, "--parts\r\nContent-Type: application/octet-stream\r\n"+
			"Content-Disposition: attachment; filename=\"part-%d.bin\"\r\n\r\npart %d\r\n", i, i)
	}
	b.WriteString("--parts--\r\n")
	return []byte(b.String())
}

// nestedParts answers a raw message of depth multipart containers, each inside the last and each closed.
func nestedParts(depth int) []byte {
	b := bytes.NewBuffer(unclosedParts(depth))
	fmt.Fprintf(b, "--level-%04d\r\nContent-Type: text/plain\r\n\r\nThe innermost part.\r\n", depth)
	for level := depth; level >= 0; level-- {
		fmt.Fprintf(b, "--level-%04d--\r\n", level)
	}
	return b.Bytes()
}

// unclosedParts answers a raw message of depth multipart containers, each inside the last and none closed.
func unclosedParts(depth int) []byte {
	var b bytes.Buffer
	b.WriteString("Content-Type: multipart/mixed; boundary=\"level-0000\"\r\n\r\n")
	for level := 1; level <= depth; level++ {
		fmt.Fprintf(&b, "--level-%04d\r\nContent-Type: multipart/mixed; boundary=\"level-%04d\"\r\n\r\n", level-1, level)
	}
	return b.Bytes()
}

// headerSummary is what TestParseLeavesMissingHeadersEmpty checks of a message.
type headerSummary struct {
	from        message.Address
	to          []message.Address
	dated       bool
	messageID   string
	threadIndex *message.ThreadIndex
}

// manyParameters answers count media type parameters, each with its own name.
func manyParameters(count int) string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("p%d=v", i)
	}
	return strings.Join(names, "; ")
}

// fileMessage answers a raw message holding the body part, then one file part with the given header fields.
func fileMessage(body, file string) string {
	return "Content-Type: multipart/related; boundary=\"files\"\r\n\r\n--files\r\n" + body +
		"\r\n--files\r\n" + file + "\r\n\r\nThe file content.\r\n--files--\r\n"
}

// fileNames answers the name of each file in files.
func fileNames(files []message.Attachment) []string {
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}
	return names
}

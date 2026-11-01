// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
)

// malformedMail is a mail ParseBytes calls malformed, with every field ReadHeaders reads.
const malformedMail = "From: Maria Perez <maria.perez@example.com>\r\nTo: info@example.com\r\n" +
	"Cc: sales@example.com\r\nBcc: desk@example.com\r\nReply-To: Maria Perez <maria.perez@example.com>\r\n" +
	"Subject: =?utf-8?q?Qu=00ote?=\r\nDate: Wed, 28 Oct 2026 07:00:00 +0000\r\n" +
	"Message-ID: <unreadable@example.com>\r\nIn-Reply-To: <quote-1@example.com>\r\n" +
	"References: <quote-1@example.com>\r\nThread-Index: AQHZ\r\nContent-Type: multipart/mixed\r\n\r\n" +
	"The boundary is missing.\r\n"

// capSender opens the mails under a header cap.
const capSender = "From: maria.perez@example.com\r\n"

// capCutCases lists raw messages, a header cap, and the sender and subject ReadHeaders answers under it.
var capCutCases = []struct {
	name    string
	raw     string
	cap     int
	from    string
	subject string
}{
	{"no cut", capSender + "Subject: Quote\r\n\r\nText.\r\n", len(capSender) + 16, "maria.perez@example.com", "Quote"},
	{
		"a block exactly at the cap with nothing after it", capSender + "Subject: Quote\r\n",
		len(capSender) + 16, "maria.perez@example.com", "Quote",
	},
	{
		"a line end one byte past the cap", capSender + "Subject: Quote\r\nTo: info@example.com\r\n\r\nText.\r\n",
		len(capSender) + 15, "maria.perez@example.com", "",
	},
	{
		"a line end ten bytes past the cap", capSender + "Subject: Quote\r\nTo: info@example.com\r\n\r\nText.\r\n",
		len(capSender) + 6, "maria.perez@example.com", "",
	},
	{
		"a cut in a line folded with a tab", capSender + "Subject: Quote\r\n\tfor the wall\r\n\r\nText.\r\n",
		len(capSender) + 22, "maria.perez@example.com", "",
	},
	{
		"a cut in a line with no fold and no colon",
		capSender + "Subject: Quote\r\nfor the wall\r\nTo: info@example.com\r\n\r\n",
		len(capSender) + 22, "maria.perez@example.com", "",
	},
	{
		"a cut on a line end before a line with no colon", capSender + "Subject: Quote\r\nfor the wall\r\n\r\nText.\r\n",
		len(capSender) + 16, "maria.perez@example.com", "",
	},
	{
		"a cut in a line opening with a carriage return", capSender + "Subject: Quote\r\n\rfor the wall\r\n\r\nText.\r\n",
		len(capSender) + 22, "maria.perez@example.com", "",
	},
	{
		"a cut past a line of one carriage return", capSender + "Subject: Quote\r\n\r\r\n for the wall\r\n\r\nText.\r\n",
		len(capSender) + 17, "maria.perez@example.com", "",
	},
	{
		"a cut past a line opening with a colon",
		capSender + "Subject: Quote\r\n: skipped\r\n for the wall\r\n\r\nText.\r\n",
		len(capSender) + 30, "maria.perez@example.com", "",
	},
	{
		"a cut past a line with a bad name", capSender + "Subject: Quote\r\nBad\x01Name: skipped\r\n for the wall\r\n\r\n",
		len(capSender) + 38, "maria.perez@example.com", "",
	},
	{
		"a cut in a mail with bare line feeds",
		"From: maria.perez@example.com\nSubject: Quote for the wall\nTo: info@example.com\n\n",
		40, "maria.perez@example.com", "",
	},
	{
		"a cut inside the value of a field",
		capSender + "Subject: Quote for the wall\r\nTo: info@example.com\r\n\r\nText.\r\n",
		len(capSender) + 14, "maria.perez@example.com", "",
	},
	{
		"a cut inside the name of a field", capSender + "Subject: Quote\r\n\r\nText.\r\n",
		len(capSender) + 4, "maria.perez@example.com", "",
	},
	{
		"a cut on a line end before a new field", capSender + "Subject: Quote\r\nTo: info@example.com\r\n\r\nText.\r\n",
		len(capSender) + 16, "maria.perez@example.com", "Quote",
	},
	{
		"a cut on a line end before a folded line", capSender + "Subject: Quote\r\n for the wall\r\n\r\nText.\r\n",
		len(capSender) + 16, "maria.perez@example.com", "",
	},
	{
		"a cut inside a folded line", capSender + "Subject: Quote\r\n for the wall\r\n\r\nText.\r\n",
		len(capSender) + 22, "maria.perez@example.com", "",
	},
	{
		"a cut inside the first field", "From: maria.perez@example.com.example.net\r\n\r\nText.\r\n",
		len("From: maria.perez@example.com"), "", "",
	},
	{
		"a cut inside a field folded from the first line", "From: maria.perez@example.com,\r\n info@example.com\r\n\r\n",
		len("From: maria.perez@example.com,\r\n info"), "", "",
	},
	{"a cut in a block with no field at all", " a stray line\r\n another stray line\r\n\r\nText.\r\n", 20, "", ""},
	{
		"a cut in a field whose name is over the line limit",
		capSender + strings.Repeat("x", 998) + ": Quote\r\n\r\nText.\r\n",
		len(capSender) + 10, "", "",
	},
	{
		"a cut in a field whose name ends at the line limit",
		capSender + strings.Repeat("x", 997) + ": Quote\r\n\r\nText.\r\n",
		len(capSender) + 10, "maria.perez@example.com", "",
	},
}

func TestReadHeadersReadsWhatParseCannot(t *testing.T) {
	t.Parallel()

	raw := []byte(malformedMail)
	if _, err := message.ParseBytes(raw, testLimits); !errors.Is(err, message.ErrMalformed) {
		t.Fatalf("ParseBytes() error = %v, want ErrMalformed", err)
	}

	m := readHeaders(t, raw, testLimits)

	want := fixtureSummary{
		from: outsideSender, to: []message.Address{bareOwnMailbox}, cc: []message.Address{{Email: "sales@example.com"}},
		bcc: []message.Address{{Email: "desk@example.com"}}, replyTo: []message.Address{outsideSender}, subject: "Quote",
		date: october(28, 7, 0), messageID: "unreadable@example.com", inReplyTo: []string{"quote-1@example.com"},
		references: []string{"quote-1@example.com"},
	}
	if got := summarize(m); !reflect.DeepEqual(got, want) {
		t.Errorf("ReadHeaders() =\n%+v\nwant\n%+v", got, want)
	}
	if m.ThreadIndex != nil {
		t.Errorf("ReadHeaders().ThreadIndex = %+v, want nil for a value too short to read", m.ThreadIndex)
	}
}

func TestReadHeadersAnswersNoBody(t *testing.T) {
	t.Parallel()

	raw := readFixture(t, "pdf-attachment.eml")

	m := readHeaders(t, raw, testLimits)

	if holdsABody(m) {
		t.Errorf("ReadHeaders() answered a body: text %q, HTML %q, files %+v, %+v",
			m.Text, m.HTML, m.Attachments, m.InlineImages)
	}
	if m.BodyCut || m.SkippedParts != 0 {
		t.Errorf("ReadHeaders() BodyCut = %v, SkippedParts = %d, want false and 0", m.BodyCut, m.SkippedParts)
	}
	if !bytes.Equal(m.Raw, raw) {
		t.Errorf("ReadHeaders().Raw = %q, want the bytes it was given", m.Raw)
	}
}

func TestReadHeadersLeavesOutAFieldTheCapCut(t *testing.T) {
	t.Parallel()

	for _, c := range capCutCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			limits := testLimits
			limits.MaxHeaderBytes = c.cap

			m := readHeaders(t, []byte(c.raw), limits)

			if m.From.Email != c.from || m.Subject != c.subject {
				t.Errorf("ReadHeaders() From = %+v, Subject = %q, want %q and %q", m.From, m.Subject, c.from, c.subject)
			}
			if m.To != nil {
				t.Errorf("ReadHeaders().To = %+v, want nothing from past the cap", m.To)
			}
		})
	}
}

func TestReadHeadersGivesNoVerdictFromACutField(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		cap  int
	}{
		{
			"a return path", capSender + "Return-Path:\r\n<sender@example.com>\r\nSubject: Quote\r\n\r\nText.\r\n",
			len(capSender) + 17,
		},
		{"a precedence", capSender + "Precedence: bulk\r\nwords\r\nSubject: Quote\r\n\r\nText.\r\n", len(capSender) + 21},
		{
			"a file name",
			capSender + "Content-Type: application/octet-stream; name=winmail.dat\r\n.txt\r\nSubject: Quote\r\n\r\n",
			len(capSender) + 60,
		},
		{
			"a method", capSender + "Content-Type: text/calendar; method=REQUEST\r\nX\r\nSubject: Quote\r\n\r\n",
			len(capSender) + 47,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			limits := testLimits
			limits.MaxHeaderBytes = c.cap

			m := readHeaders(t, []byte(c.raw), limits)

			if got := message.Classify(m); !reflect.DeepEqual(got, message.Classification{}) {
				t.Errorf("Classify() = %s, want nothing", describe(got))
			}
			if got := message.Mark(m); got != (message.Marks{}) {
				t.Errorf("Mark() = %+v, want nothing", got)
			}
		})
	}
}

func TestReadHeadersReadsAMediaFieldAtTheFieldCap(t *testing.T) {
	t.Parallel()

	const value = `attachment; filename="winmail.dat"`
	raw := []byte("Content-Disposition: " + value + "\r\n\r\nBody.\r\n")
	cases := []struct {
		maxFieldBytes int
		want          message.Marks
	}{
		{len(value), message.Marks{Winmail: true}},
		{len(value) - 1, message.Marks{}},
	}

	for _, c := range cases {
		limits := testLimits
		limits.MaxFieldBytes = c.maxFieldBytes

		if got := message.Mark(readHeaders(t, raw, limits)); got != c.want {
			t.Errorf("Mark() under a field cap of %d = %+v, want %+v", c.maxFieldBytes, got, c.want)
		}
	}
}

func TestReadHeadersFeedsClassifyAndMark(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		field    string
		classify message.Classification
		marks    message.Marks
	}{
		{"a plain mail", "Content-Type: text/plain", message.Classification{}, message.Marks{}},
		{
			"a list mail", "List-Id: <news.example.com>",
			message.Classification{Automation: message.ListMail}, message.Marks{},
		},
		{
			"a bounce", "Content-Type: multipart/report; report-type=delivery-status",
			message.Classification{Automation: message.Bounce, Report: &message.DeliveryReport{}}, message.Marks{},
		},
		{
			"an encrypted mail", "Content-Type: multipart/encrypted; protocol=\"application/pgp-encrypted\"",
			message.Classification{}, message.Marks{Encrypted: true},
		},
		{"a signed mail", "Content-Type: multipart/signed", message.Classification{}, message.Marks{Signed: true}},
		{
			"an invite", "Content-Type: text/calendar; method=REQUEST",
			message.Classification{}, message.Marks{InviteMethod: "REQUEST"},
		},
		{"a type that cannot be read", "Content-Type: a b", message.Classification{}, message.Marks{}},
		{
			"a TNEF type", "Content-Type: application/ms-tnef",
			message.Classification{}, message.Marks{Winmail: true},
		},
		{
			"a TNEF file named by its disposition",
			"Content-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=\"winmail.dat\"",
			message.Classification{}, message.Marks{Winmail: true},
		},
		{
			"a TNEF file named by its type", "Content-Type: application/octet-stream; name=\"WIN.DAT\"",
			message.Classification{}, message.Marks{Winmail: true},
		},
		{
			"a TNEF file named by the old type parameter", "Content-Type: application/octet-stream; file=\"winmail.dat\"",
			message.Classification{}, message.Marks{Winmail: true},
		},
		{
			"a disposition naming another file than the type",
			"Content-Type: application/octet-stream; name=\"winmail.dat\"\r\n" +
				"Content-Disposition: attachment; filename=\"quote.pdf\"",
			message.Classification{}, message.Marks{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			m := readHeaders(t, []byte(c.field+"\r\n\r\nBody.\r\n"), testLimits)

			if got := message.Classify(m); !reflect.DeepEqual(got, c.classify) {
				t.Errorf("Classify() = %s, want %s", describe(got), describe(c.classify))
			}
			if got := message.Mark(m); got != c.marks {
				t.Errorf("Mark() = %+v, want %+v", got, c.marks)
			}
		})
	}
}

func TestReadHeadersDropsALongMediaFieldQuickly(t *testing.T) {
	cases := []struct {
		name  string
		field string
	}{
		{"a type field", "Content-Type: text/calendar; method=REQUEST; " + manyParameters()},
		{"a disposition field", "Content-Disposition: attachment; " + manyParameters()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := []byte("From: maria.perez@example.com\r\n" + c.field + "\r\n\r\nText.\r\n")
			limits := testLimits
			limits.MaxHeaderBytes = len(raw)

			m, marks := readAndMarkWithinDeadline(t, raw, limits)

			if m.From.Email != "maria.perez@example.com" || marks != (message.Marks{}) {
				t.Errorf("ReadHeaders() From = %+v, Mark() = %+v, want the sender and no marks", m.From, marks)
			}
		})
	}
}

func TestReadHeadersTakesWhatItCanRead(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
	}{
		{"nothing", ""},
		{"a blank line", "\r\n"},
		{"no field at all", "No field here\r\n"},
		{"a line opening with a colon", ": no name\r\n\r\nText.\r\n"},
		{"a line opening with a blank", " a stray line\r\n\r\nText.\r\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			m := readHeaders(t, []byte(c.raw), testLimits)

			if got := headersOf(m); !reflect.DeepEqual(got, headerSummary{}) {
				t.Errorf("ReadHeaders(%q) headers = %+v, want no sender, recipient, date, id or thread index", c.raw, got)
			}
			if m.Subject != "" {
				t.Errorf("ReadHeaders(%q).Subject = %q, want none", c.raw, m.Subject)
			}
		})
	}
}

func TestReadHeadersChecksItsLimits(t *testing.T) {
	t.Parallel()

	m, err := message.ReadHeaders([]byte(malformedMail), message.Limits{})

	if !errors.Is(err, message.ErrLimitsUnset) || m != nil {
		t.Errorf("ReadHeaders() with no limits = %+v, %v, want nil and ErrLimitsUnset", m, err)
	}
}

// holdsABody reports whether m answers a body or a file.
func holdsABody(m *message.Message) bool {
	return m.Text != "" || m.HTML != "" || m.Attachments != nil || m.InlineImages != nil
}

// readHeaders answers the headers of raw under limits, failing t on an error.
func readHeaders(t *testing.T, raw []byte, limits message.Limits) *message.Message {
	t.Helper()
	m, err := message.ReadHeaders(raw, limits)
	if err != nil {
		t.Fatalf("ReadHeaders() error = %v, want nil", err)
	}
	return m
}

// readAndMarkWithinDeadline answers ReadHeaders and Mark of raw, failing t when they run past deadline.
func readAndMarkWithinDeadline(t *testing.T, raw []byte, limits message.Limits) (*message.Message, message.Marks) {
	t.Helper()
	type result struct {
		m     *message.Message
		marks message.Marks
	}
	done := make(chan result, 1)
	go func() {
		m, _ := message.ReadHeaders(raw, limits)
		done <- result{m, message.Mark(m)}
	}()
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.m, r.marks
	case <-timer.C:
		t.Fatalf("ReadHeaders() and Mark() of %d bytes took over %v", len(raw), deadline)
		return nil, message.Marks{}
	}
}

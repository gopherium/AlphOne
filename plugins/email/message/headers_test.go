// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
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

func TestReadHeadersReadsWhatParseCannot(t *testing.T) {
	t.Parallel()

	raw := []byte(malformedMail)
	if _, err := message.ParseBytes(raw, testLimits); !errors.Is(err, message.ErrMalformed) {
		t.Fatalf("ParseBytes() error = %v, want ErrMalformed", err)
	}

	m := readHeaders(t, raw, testLimits)

	if m.From != outsideSender || !slices.Equal(m.ReplyTo, []message.Address{outsideSender}) {
		t.Errorf("ReadHeaders() From = %+v, ReplyTo = %+v, want %+v in both", m.From, m.ReplyTo, outsideSender)
	}
	if !slices.Equal(m.To, []message.Address{bareOwnMailbox}) {
		t.Errorf("ReadHeaders().To = %+v, want %+v", m.To, bareOwnMailbox)
	}
	if want := []message.Address{{Email: "sales@example.com"}}; !slices.Equal(m.Cc, want) {
		t.Errorf("ReadHeaders().Cc = %+v, want %+v", m.Cc, want)
	}
	if want := []message.Address{{Email: "desk@example.com"}}; !slices.Equal(m.Bcc, want) {
		t.Errorf("ReadHeaders().Bcc = %+v, want %+v", m.Bcc, want)
	}
	if m.Subject != "Quote" {
		t.Errorf("ReadHeaders().Subject = %q, want Quote", m.Subject)
	}
	if want := october(28, 7, 0); !m.Date.Equal(want) {
		t.Errorf("ReadHeaders().Date = %v, want %v", m.Date, want)
	}
	if m.MessageID != "unreadable@example.com" || !slices.Equal(m.InReplyTo, []string{"quote-1@example.com"}) ||
		!slices.Equal(m.References, []string{"quote-1@example.com"}) {
		t.Errorf("ReadHeaders() ids = %q, %q, %q", m.MessageID, m.InReplyTo, m.References)
	}
	if m.ThreadIndex != nil {
		t.Errorf("ReadHeaders().ThreadIndex = %+v, want nil for a value too short to read", m.ThreadIndex)
	}
}

func TestReadHeadersAnswersNoBody(t *testing.T) {
	t.Parallel()

	raw := readFixture(t, "pdf-attachment.eml")

	m := readHeaders(t, raw, testLimits)

	if m.Text != "" || m.HTML != "" || m.Attachments != nil || m.InlineImages != nil {
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

func TestReadHeadersStopsAtTheHeaderCap(t *testing.T) {
	t.Parallel()

	const opening = "From: maria.perez@example.com\r\nSubject: Quote"
	limits := testLimits
	limits.MaxHeaderBytes = len(opening)
	cases := []struct {
		name    string
		raw     string
		subject string
	}{
		{"a field cut in the middle", opening + " for the new facade\r\nTo: info@example.com\r\n\r\nText.\r\n", "Quote"},
		{
			"a field cut inside a letter",
			strings.Replace(opening, "Quote", "Quot\xc3", 1) + "\xa9\r\nTo: info@example.com\r\n\r\nText.\r\n",
			"Quot" + replacement,
		},
		{"a field ending at the cap", opening + "\r\nTo: info@example.com\r\n\r\nText.\r\n", "Quote"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			m := readHeaders(t, []byte(c.raw), limits)

			if m.From.Email != "maria.perez@example.com" || m.Subject != c.subject {
				t.Errorf("ReadHeaders() From = %+v, Subject = %q, want the sender and %q", m.From, m.Subject, c.subject)
			}
			if m.To != nil {
				t.Errorf("ReadHeaders().To = %+v, want nothing from past the cap", m.To)
			}
		})
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

			if m.From != (message.Address{}) || m.Subject != "" || m.To != nil {
				t.Errorf("ReadHeaders(%q) = %+v, want no sender, subject or recipient", c.raw, m)
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

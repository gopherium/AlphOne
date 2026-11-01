// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"net/mail"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gopherium/alphone/plugins/email/message"
)

// fuzzSeeds lists raw messages that reach the paths a fixture does not.
var fuzzSeeds = []string{
	"Subject: =?utf-8?q?Qu=00ote?=\r\n\r\nText.\r\n",
	"From: =?utf-8?q?Maria=00?= <maria.perez@example.com>\r\nTo: =?utf-8?q?a=00b?=@example.com, info@example.com\r\n\r\n",
	"From: Maria Perez \xa9 Lamps <maria.perez@example.com>\r\nCc: info@example.com, ??? junk\r\n\r\nText.\r\n",
	"Content-Type: text/calendar; method*=utf-8''RE%00QUEST\r\n\r\nBEGIN:VCALENDAR\r\n",
	onePart("multipart/mixed; boundary=\"p\"", "--p\r\nContent-Type: application/x-foo bar\r\n\r\nLost.\r\n--p--"),
	"Content-Type: multipart/mixed\r\n\r\nThe boundary is missing.\r\n",
	"Content-Type: text/html; charset=utf-8\r\n\r\n<p>Te\xffxt\x00.</p>\r\n",
	onePart("multipart/report; report-type=delivery-status; boundary=\"r\"",
		"--r\r\nContent-Type: message/delivery-status\r\n\r\nReporting-MTA: dns; mail.example.com\r\n\r\n"+
			"Final-Recipient: rfc822; no\xffbody@example.com\r\nAction: failed\r\nStatus: 5.1\xff.1\r\n--r--"),
}

func FuzzParseAddressesAnswersOneMailboxEach(f *testing.F) {
	for _, c := range parseAddressesCases {
		f.Add(c.value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		for _, a := range message.ParseAddresses(value) {
			checkCleanText(t, "address name", a.Name)
			checkCleanText(t, "address email", a.Email)
			if read, ok := message.NormalizeAddress(mail.Address{Address: a.Email}); !ok || read.Email != a.Email {
				t.Fatalf("ParseAddresses(%q) answered %q, which does not read back as itself", value, a.Email)
			}
			if _, err := mail.ParseAddress(a.Email); err != nil {
				t.Fatalf("ParseAddresses(%q) answered %q, which is no mailbox: %v", value, a.Email, err)
			}
		}
	})
}

func FuzzParseAnswersCleanStrings(f *testing.F) {
	for _, name := range fixtureNames(f) {
		f.Add(readFixture(f, name))
	}
	for _, seed := range fuzzSeeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if m, err := message.ParseBytes(raw, testLimits); err == nil {
			checkCleanStrings(t, m)
		}
		m, err := message.ReadHeaders(raw, testLimits)
		if err != nil {
			t.Fatalf("ReadHeaders() error = %v, want nil", err)
		}
		checkCleanStrings(t, m)
	})
}

// checkCleanStrings fails t when a string m or the calls reading it answer is broken UTF-8 or holds a NUL.
func checkCleanStrings(t *testing.T, m *message.Message) {
	t.Helper()
	snippet, err := message.Snippet(m.Text, 200)
	if err != nil {
		t.Fatalf("Snippet() error = %v, want nil", err)
	}
	texts := map[string]string{
		"Subject": m.Subject, "Text": m.Text, "HTML": m.HTML, "MessageID": m.MessageID, "Snippet": snippet,
		"InviteMethod": message.Mark(m).InviteMethod, "Sanitize": message.Sanitize(m.HTML, nil),
	}
	for name, text := range texts {
		checkCleanText(t, name, text)
	}
	for _, id := range append(m.InReplyTo, m.References...) {
		checkCleanText(t, "id", id)
	}
	for _, a := range append([]message.Address{m.From}, addressesOf(m)...) {
		checkCleanText(t, "address name", a.Name)
		checkCleanText(t, "address email", a.Email)
	}
	for _, file := range append(m.Attachments, m.InlineImages...) {
		checkCleanText(t, "file name", file.Name)
		checkCleanText(t, "file type", file.ContentType)
		checkCleanText(t, "file id", file.ContentID)
	}
	if report := message.Classify(m).Report; report != nil {
		for _, failed := range report.Failed {
			checkCleanText(t, "failed email", failed.Email)
			checkCleanText(t, "failed status", failed.Status)
		}
	}
}

// addressesOf answers every recipient and reply address of m.
func addressesOf(m *message.Message) []message.Address {
	var all []message.Address
	for _, list := range [][]message.Address{m.To, m.Cc, m.Bcc, m.ReplyTo} {
		all = append(all, list...)
	}
	return all
}

// checkCleanText fails t when text is broken UTF-8 or holds a NUL.
func checkCleanText(t *testing.T, name, text string) {
	t.Helper()
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		t.Fatalf("%s = %q, want UTF-8 with no NUL", name, text)
	}
}

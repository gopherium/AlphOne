// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"crypto/sha256"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
)

// testLimits are the limits the tests read mail within, the default of every parse setting.
var testLimits = message.Limits{
	MaxBytes: defaultMessageMaxBytes, MaxParts: defaultMessageMaxParts, MaxContainers: defaultMessageMaxContainers,
	MaxFieldBytes: defaultMessageMaxFieldBytes, MaxHeaderBytes: defaultHeaderMaxBytes, MaxBodyBytes: defaultBodyMaxBytes,
}

// quoteDate is the Date field of the test mails.
const quoteDate = "Mon, 02 Nov 2026 09:00:00 +0000"

// quoteSeconds is the time quoteDate names, in Unix seconds.
var quoteSeconds = strconv.FormatInt(time.Date(2026, time.November, 2, 9, 0, 0, 0, time.UTC).Unix(), 10)

// rawMail joins lines into a raw mail with the line ends the mail standard asks for.
func rawMail(lines ...string) []byte {
	return []byte(strings.Join(lines, "\r\n") + "\r\n")
}

// parsed reads raw within testLimits, failing the test when it does not read.
func parsed(t *testing.T, raw []byte) *message.Message {
	t.Helper()
	m, err := message.ParseBytes(raw, testLimits)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v, want nil", err)
	}
	return m
}

// quoteMail returns the mail asking for a quote, with header lines put before its own and the body text given.
func quoteMail(before []string, text string) []byte {
	return rawMail(append(before,
		"From: Maria Perez <maria.perez@example.com>",
		"To: info@example.com",
		"Subject: Your quote",
		"Date: "+quoteDate,
		"Message-ID: <quote@example.com>",
		"Content-Type: text/plain",
		"",
		text,
	)...)
}

// invoiceMail returns a mail with the id invoice@example.com that carries each file content as a PDF.
func invoiceMail(contents ...string) []byte {
	lines := []string{
		"From: Maria Perez <maria.perez@example.com>",
		"To: info@example.com",
		"Subject: Invoice",
		"Date: " + quoteDate,
		"Message-ID: <invoice@example.com>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="files"`,
		"",
		"--files",
		"Content-Type: text/plain",
		"",
		"Here is the invoice.",
	}
	for _, content := range contents {
		lines = append(lines, "--files", "Content-Type: application/pdf",
			`Content-Disposition: attachment; filename="invoice.pdf"`, "", content)
	}
	return rawMail(append(lines, "--files--")...)
}

// sum returns the SHA-256 of text.
func sum(text string) []byte {
	hash := sha256.Sum256([]byte(text))
	return hash[:]
}

func TestDedupeKeyOfAMailWithAnIDHashesItsIDSenderDateAndContent(t *testing.T) {
	t.Parallel()

	m := parsed(t, quoteMail(nil, "Could you send me a quote for the new lamp?"))

	got := dedupeKey(m, "file:inbox")

	want := hashParts("id", []byte("quote@example.com"), []byte("maria.perez@example.com"), []byte(quoteSeconds),
		[]byte("your quote"), sum("Could you send me a quote for the new lamp?"))
	if !bytes.Equal(got, want) {
		t.Errorf("dedupeKey() = %x, want the id form %x", got, want)
	}
}

func TestDedupeKeyOfAMailWithAnIDAndNoDateHashesAnEmptyDate(t *testing.T) {
	t.Parallel()

	m := parsed(t, rawMail("From: maria.perez@example.com", "Subject: Your quote", "Message-ID: <quote@example.com>",
		"Content-Type: text/plain", "", "Could you send me a quote?"))

	got := dedupeKey(m, "file:inbox")

	want := hashParts("id", []byte("quote@example.com"), []byte("maria.perez@example.com"), nil,
		[]byte("your quote"), sum("Could you send me a quote?"))
	if !bytes.Equal(got, want) {
		t.Errorf("dedupeKey() = %x, want the id form with an empty date %x", got, want)
	}
}

func TestDedupeKeyOfAMailWithNoIDHashesItsSenderDateAndContent(t *testing.T) {
	t.Parallel()

	m := parsed(t, rawMail("From: maria.perez@example.com", "Subject: Your quote", "Date: "+quoteDate,
		"Content-Type: text/plain", "", "Could you send me a quote?"))

	got := dedupeKey(m, "file:inbox")

	want := hashParts("noid", []byte("maria.perez@example.com"), []byte(quoteSeconds), []byte("your quote"),
		sum("Could you send me a quote?"))
	if !bytes.Equal(got, want) {
		t.Errorf("dedupeKey() = %x, want the noid form %x", got, want)
	}
}

func TestDedupeKeyOfAMailWithNoIDAndNoDateHashesItsProviderRef(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string][]byte{
		"no date":   rawMail("From: maria.perez@example.com", "Content-Type: text/plain", "", "Hello"),
		"no sender": rawMail("Date: "+quoteDate, "Content-Type: text/plain", "", "Hello"),
	} {
		m := parsed(t, raw)

		inbox, sent := dedupeKey(m, "file:inbox"), dedupeKey(m, "file:sent")

		if want := hashParts("ref", []byte("file:inbox")); !bytes.Equal(inbox, want) {
			t.Errorf("dedupeKey() of a mail with %s = %x, want the ref form %x", name, inbox, want)
		}
		if bytes.Equal(inbox, sent) {
			t.Errorf("two copies of a mail with %s under two references share the key %x, want two keys", name, inbox)
		}
	}
}

func TestTheLeadingWordKeepsTheFormsApart(t *testing.T) {
	t.Parallel()

	parts := [][]byte{[]byte("maria.perez@example.com"), []byte(quoteSeconds)}

	keys := map[string]string{}
	for _, word := range []string{"id", "noid", "ref"} {
		keys[string(hashParts(word, parts...))] = word
	}

	if len(keys) != 3 {
		t.Errorf("the three forms over the same parts answer %d keys, want 3", len(keys))
	}
}

func TestLengthPrefixesKeepOnePartFromRunningIntoTheNext(t *testing.T) {
	t.Parallel()

	split, joined := hashParts("ref", []byte("ab"), []byte("c")), hashParts("ref", []byte("a"), []byte("bc"))

	if bytes.Equal(split, joined) {
		t.Errorf("ab then c and a then bc share the key %x, want two keys", split)
	}
}

func TestTwoCopiesWithOtherReceivedLinesMatch(t *testing.T) {
	t.Parallel()

	inbox := parsed(t, quoteMail([]string{"Received: from mx1.example.com by mx2.example.com"}, "Could you send it?"))
	sent := parsed(t, quoteMail([]string{"Received: from relay.example.com by mx3.example.com",
		"Received: from mx4.example.com by relay.example.com"}, "Could you send it?"))

	if a, b := dedupeKey(inbox, "file:inbox"), dedupeKey(sent, "file:sent"); !bytes.Equal(a, b) {
		t.Errorf("two copies with other Received lines answer %x and %x, want one key", a, b)
	}
}

func TestACopyWithOtherLineEndsAndBlanksMatches(t *testing.T) {
	t.Parallel()

	crlf := parsed(t, quoteMail(nil, "Could you send\r\nthe quote?"))
	lf := parsed(t, []byte(strings.ReplaceAll(string(quoteMail(nil, "Could  you send\r\n\r\nthe quote? ")), "\r\n", "\n")))

	if a, b := dedupeKey(crlf, "file:inbox"), dedupeKey(lf, "file:sent"); !bytes.Equal(a, b) {
		t.Errorf("a copy with other line ends and blanks answers %x beside %x, want one key", b, a)
	}
}

func TestTwoMailsWithOneIDAndOtherTextDiffer(t *testing.T) {
	t.Parallel()

	first := parsed(t, quoteMail(nil, "Here is the invoice for March."))
	second := parsed(t, quoteMail(nil, "Here is the invoice for April."))

	if a := dedupeKey(first, "file:march"); bytes.Equal(a, dedupeKey(second, "file:april")) {
		t.Errorf("two mails with one id and other text share the key %x, want two keys", a)
	}
}

func TestTwoMailsWithOneIDAndOtherAttachmentsDiffer(t *testing.T) {
	t.Parallel()

	march, april := parsed(t, invoiceMail("March invoice")), parsed(t, invoiceMail("April invoice"))

	if a := dedupeKey(march, "file:march"); bytes.Equal(a, dedupeKey(april, "file:april")) {
		t.Errorf("two mails with one id and other attachments share the key %x, want two keys", a)
	}
}

func TestTwoCopiesWithTheirFilesInAnotherOrderMatch(t *testing.T) {
	t.Parallel()

	first, second := parsed(t, invoiceMail("March", "April")), parsed(t, invoiceMail("April", "March"))

	if a, b := dedupeKey(first, "file:inbox"), dedupeKey(second, "file:sent"); !bytes.Equal(a, b) {
		t.Errorf("two copies with their files in another order answer %x and %x, want one key", a, b)
	}
}

func TestNormalizedSubjectLowercasesTheSubjectWithoutItsTags(t *testing.T) {
	t.Parallel()

	for _, subject := range []string{"INVOICE", "Re: Invoice", "Fwd:  invoice "} {
		if got := normalizedSubject(subject); got != "invoice" {
			t.Errorf("normalizedSubject(%q) = %q, want invoice", subject, got)
		}
	}
}

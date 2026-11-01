// SPDX-License-Identifier: Elastic-2.0

//go:build !race

package message_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/gopherium/alphone/plugins/email/message"
)

// mailBytes is the size of the largest mail a server takes.
const mailBytes = 34 << 20

// memoryBudget is the most one mail may allocate in all on its way through the package.
const memoryBudget = 1 << 30

// serverLimits are the limits a server takes mail under.
var serverLimits = message.Limits{
	MaxBytes: 35 << 20, MaxParts: 200, MaxContainers: 16, MaxFieldBytes: 4096,
	MaxHeaderBytes: 256 << 10, MaxBodyBytes: 4 << 20,
}

func TestParseStaysInsideTheMemoryBudget(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"an HTML body", "Content-Type: text/html\r\n\r\n" + filled("<p>The quote for the new wall.</p>\r\n")},
		{"an HTML body of list items", "Content-Type: text/html\r\n\r\n<ul>" + filled("<li>a")},
		{"an HTML body of links", "Content-Type: text/html\r\n\r\n" + filled(`<a href="https://example.com/">a</a>`)},
		{"a text body", "Content-Type: text/plain\r\n\r\n" + filled("The quote for the new wall.\r\n")},
		{
			"one attachment",
			"Content-Type: multipart/mixed; boundary=\"file\"\r\n\r\n--file\r\nContent-Type: application/pdf\r\n" +
				"Content-Disposition: attachment; filename=\"quote.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
				filled(strings.Repeat("QUJD", 19)+"\r\n") + "--file--\r\n",
		},
		{"one long subject", "Subject: " + filled("quote ") + "\r\n\r\nText.\r\n"},
		{"many recipients", "To: " + filled("a@example.com, ") + "b@example.com\r\n\r\nText.\r\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := []byte(c.raw)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			readWholeMail(t, raw)
			runtime.ReadMemStats(&after)

			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > memoryBudget {
				t.Errorf("reading %d bytes allocated %d MiB, want at most %d MiB", len(raw), allocated>>20, memoryBudget>>20)
			}
		})
	}
}

func TestReadHeadersCostsTheSameForAnyLengthOfCutField(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{"a long field", []byte("From: maria.perez@example.com\r\nSubject: " + filled("quote ") + "\r\n\r\nText.\r\n")},
		{"a long line with no colon", []byte("From: maria.perez@example.com\r\n" + filled("quote ") + "\r\n\r\nText.\r\n")},
		{"a long field name", []byte("From: maria.perez@example.com\r\n" + filled("x") + ": Quote\r\n\r\nText.\r\n")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			limits := serverLimits
			limits.MaxBytes = 1 << 30
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			readHeaders(t, c.raw, limits)
			runtime.ReadMemStats(&after)

			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
				t.Errorf("reading the headers of %d bytes allocated %d KiB, want at most 1 MiB", len(c.raw), allocated>>10)
			}
		})
	}
}

// filled answers line repeated up to mailBytes.
func filled(line string) string {
	return strings.Repeat(line, mailBytes/len(line))
}

// readWholeMail runs raw through every step a stored mail takes, falling back to its headers when it cannot be parsed.
func readWholeMail(t *testing.T, raw []byte) {
	t.Helper()
	m, err := message.ParseBytes(raw, serverLimits)
	if err != nil {
		m = readHeaders(t, raw, serverLimits)
	}
	message.Classify(m)
	message.Mark(m)
	message.Sanitize(m.HTML, nil)
	if _, err := message.Snippet(m.Text, 200); err != nil {
		t.Fatalf("Snippet() error = %v, want nil", err)
	}
}

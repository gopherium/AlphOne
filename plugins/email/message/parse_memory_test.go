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

// serverBodyCap is the body cap a server takes mail under.
const serverBodyCap = 4 << 20

// htmlMemoryBudget is the most one HTML mail may allocate in all, sixteen times the mail and the body cap together.
const htmlMemoryBudget = 16 * (mailBytes + serverBodyCap)

// serverLimits are the limits a server takes mail under.
var serverLimits = message.Limits{
	MaxBytes: 35 << 20, MaxParts: 200, MaxContainers: 16, MaxFieldBytes: 4096,
	MaxHeaderBytes: 256 << 10, MaxBodyBytes: serverBodyCap,
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

func TestParseKeepsAnHTMLMailInsideASmallMultipleOfItsSize(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"deeply nested quotes", "Content-Type: text/html\r\n\r\n" + filled("<blockquote>Quote<br>")},
		{
			"many lines inside 500 quotes",
			"Content-Type: text/html\r\n\r\n" + strings.Repeat("<blockquote>", 500) + filled("Quote<br>"),
		},
		{"nested bold text", "Content-Type: text/html\r\n\r\n" + filled("<b>Quote ")},
		{"words inside 500 bold elements", "Content-Type: text/html\r\n\r\n" + strings.Repeat("<b>", 500) + filled("Quote ")},
		{"nested headings", "Content-Type: text/html\r\n\r\n" + filled("<h1>Quote ")},
		{"nested lists", "Content-Type: text/html\r\n\r\n" + filled("<ul><li>Quote ")},
		{"nested table cells", "Content-Type: text/html\r\n\r\n" + filled("<table><tr><td>Quote ")},
		{"one long text with entities", "Content-Type: text/html\r\n\r\n" + filled("&amp;&lt;&#233;&eacute; ")},
		{"one long text of quote marks", "Content-Type: text/html\r\n\r\n" + filled(`"`)},
		{
			"one long text of escaped marks and letters a meta charset widens",
			"Content-Type: text/html\r\n\r\n<meta charset=\"windows-874\"><p>" + filled("\"&\xa1"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := []byte(c.raw)
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			readWholeMail(t, raw)
			runtime.ReadMemStats(&after)

			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > htmlMemoryBudget {
				t.Errorf("reading %d bytes of HTML allocated %d MiB, want at most %d MiB", len(raw), allocated>>20,
					htmlMemoryBudget>>20)
			}
		})
	}
}

func TestTextFromHTMLStopsReadingAtTheLimit(t *testing.T) {
	document := filled("<p>The quote for the new wall.</p>\r\n")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	text, cut := message.TextFromHTML(document, 16)

	runtime.ReadMemStats(&after)
	if text != "The quote for th" || !cut {
		t.Errorf("TextFromHTML() = %q, %t, want %q, true", text, cut, "The quote for th")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Errorf("TextFromHTML() of %d bytes up to 16 allocated %d KiB, want at most 1 MiB", len(document),
			allocated>>10)
	}
}

func TestTextFromHTMLStopsCleaningAtTheLimit(t *testing.T) {
	valid, broken := filled("Quaote abc "), filled("Qu\x00ote \xff\xfe\xc3 ")

	validAllocated := allocatedBy(func() { message.TextFromHTML(valid, 16) })
	brokenAllocated := allocatedBy(func() { message.TextFromHTML(broken, 16) })

	if brokenAllocated > validAllocated+1<<20 {
		t.Errorf("TextFromHTML() of %d bytes of broken text up to 16 allocated %d KiB more than of valid text, "+
			"want at most 1 MiB more", len(broken), (brokenAllocated-validAllocated)>>10)
	}
}

func TestTextFromHTMLOfNestedSVGElementsAllocatesAsMuchAsOfNestedHTMLElements(t *testing.T) {
	nestedSVG, nestedHTML := filled("<svg><g>"), filled("<div><b>")

	svgAllocated := allocatedBy(func() { message.TextFromHTML(nestedSVG, 16) })
	htmlAllocated := allocatedBy(func() { message.TextFromHTML(nestedHTML, 16) })

	if svgAllocated > htmlAllocated+1<<20 {
		t.Errorf("TextFromHTML() of %d bytes of nested SVG elements allocated %d KiB more than of nested HTML elements, "+
			"want at most 1 MiB more", len(nestedSVG), (svgAllocated-htmlAllocated)>>10)
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

// allocatedBy answers the bytes run allocates.
func allocatedBy(run func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	run()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
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
	if _, err := message.Snippet(m.Text, 200); err != nil {
		t.Fatalf("Snippet() error = %v, want nil", err)
	}
}

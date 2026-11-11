// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// sampleAddress is the address of the sample mailbox the seed stores.
const sampleAddress = "info@example.com"

// sampleAlias is the other address that reaches the sample mailbox.
const sampleAlias = "sales@example.com"

// base64LineLetters is the most letters one line of a base64 body holds, the length MIME sets.
const base64LineLetters = 76

// lampPNG is the one pixel image, in base64, the sample mail shows inline.
const lampPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// quoteText is the content stream of the one page of the sample quote.
const quoteText = "BT /F1 18 Tf 72 770 Td (Quote for the new lamp) Tj /F1 12 Tf 0 -32 Td " +
	"(Wicker lamp, one piece, 120 EUR) Tj 0 -18 Td (Delivery within two weeks) Tj ET"

// sampleMail is one mail of the sample mailbox, beside the reference it is stored under.
type sampleMail struct {
	// ref is the provider reference the mail is stored under.
	ref string
	// age is how long before the seed the mailbox received the mail.
	age time.Duration
	// sentCopy marks the reply read from the Sent folder.
	sentCopy bool
	// header holds the header lines of the mail but its Date.
	header []string
	// body holds the lines after the header.
	body []string
}

// raw returns the mail dated at, every line ended the way mail ends it.
func (m sampleMail) raw(at time.Time) []byte {
	lines := slices.Concat(m.header, []string{"Date: " + at.Format(time.RFC1123Z), ""}, m.body)
	return []byte(strings.Join(lines, "\r\n") + "\r\n")
}

// sampleMails returns the mails of the sample mailbox in the order the mailbox received them.
func sampleMails() []sampleMail {
	return []sampleMail{
		{ref: "seed:lamp-quote", age: 72 * time.Hour, header: []string{
			"From: Maria Perez <maria.perez@example.com>", "To: Info <info@example.com>",
			"Subject: Quote for the new lamp", "Message-ID: <seed-lamp-quote@example.com>", "MIME-Version: 1.0",
			"Content-Type: text/html; charset=utf-8",
		}, body: []string{
			"<p>Hello,</p>", "<p>Could you send me a quote for the new wicker lamp?</p>", "<p>Maria Perez</p>",
		}},
		{ref: "seed:lamp-quote-reply", age: 70 * time.Hour, sentCopy: true, header: []string{
			"From: Info <info@example.com>", "To: Maria Perez <maria.perez@example.com>",
			"Subject: Re: Quote for the new lamp", "Message-ID: <seed-lamp-quote-reply@example.com>",
			"In-Reply-To: <seed-lamp-quote@example.com>", "References: <seed-lamp-quote@example.com>",
			"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="quote"`,
		}, body: slices.Concat([]string{
			"--quote", "Content-Type: text/plain; charset=utf-8", "",
			"Hello Maria, the quote for the new lamp is attached.",
			"--quote", "Content-Type: application/pdf", `Content-Disposition: attachment; filename="quote.pdf"`,
			"Content-Transfer-Encoding: base64", "",
		}, base64Lines(quotePDF()), []string{"--quote--"})},
		{ref: "seed:lamp-photo", age: 48 * time.Hour, header: []string{
			"From: Maria Perez <maria.perez@example.com>", "To: Info <info@example.com>",
			"Subject: Re: Quote for the new lamp", "Message-ID: <seed-lamp-photo@example.com>",
			"In-Reply-To: <seed-lamp-quote-reply@example.com>",
			"References: <seed-lamp-quote@example.com> <seed-lamp-quote-reply@example.com>",
			"MIME-Version: 1.0", `Content-Type: multipart/related; boundary="photo"`,
		}, body: []string{
			"--photo", "Content-Type: text/html; charset=utf-8", "",
			`<p>Thank you. Is it this model?</p><p><img src="cid:lamp-photo@example.com" alt="The wicker lamp"></p>`,
			"--photo", "Content-Type: image/png", "Content-ID: <lamp-photo@example.com>",
			`Content-Disposition: inline; filename="lamp.png"`, "Content-Transfer-Encoding: base64", "",
			lampPNG, "--photo--",
		}},
		{ref: "seed:site-visit", age: 24 * time.Hour, header: []string{
			"From: Maria Perez <maria.perez@example.com>", "To: Info <info@example.com>",
			"Cc: Ada Lovelace <ada@example.com>", "Subject: Site visit on Friday",
			"Message-ID: <seed-site-visit@example.com>", "Content-Type: text/plain; charset=utf-8",
		}, body: []string{"Ada and I can meet you at the shop on Friday at ten to choose the shades."}},
		{ref: "seed:delivery-date", age: 2 * time.Hour, header: []string{
			"From: Ada Lovelace <ada@example.com>", "To: Info <info@example.com>", "Subject: Delivery date",
			"Message-ID: <seed-delivery-date@example.com>", "Content-Type: text/plain; charset=utf-8",
		}, body: []string{"When will the lamps reach the shop?"}},
	}
}

// base64Lines returns content in base64, cut into lines of base64LineLetters letters.
func base64Lines(content []byte) []string {
	var lines []string
	for line := range slices.Chunk([]byte(base64.StdEncoding.EncodeToString(content)), base64LineLetters) {
		lines = append(lines, string(line))
	}
	return lines
}

// quotePDF returns the one page PDF of the sample quote.
func quotePDF() []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R " +
			"/Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(quoteText), quoteText),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	table := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, table)
	return pdf.Bytes()
}

// seed stores the sample mailbox of the workspace tenant and its mail, leaving every sample mail stored before alone.
func (p *ingester) seed(ctx context.Context, tenant uuid.UUID) error {
	box, err := ensureMailbox(ctx, p.pool, tenant, sampleAddress, []string{sampleAlias})
	if err != nil {
		return err
	}
	now := p.now().UTC().Truncate(time.Second)
	for _, sample := range sampleMails() {
		received := now.Add(-sample.age)
		_, _, err := p.store(ctx, tenant, Incoming{
			MailboxID: box.id, Raw: bytes.NewReader(sample.raw(received)), ProviderRef: sample.ref,
			ReceivedAt: received, SentCopy: sample.sentCopy,
		})
		if err != nil {
			return fmt.Errorf("email: seed the sample mail %s: %w", sample.ref, err)
		}
	}
	return nil
}

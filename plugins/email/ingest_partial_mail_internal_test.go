// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// brokenInvoiceMail returns a mail from Maria Perez with a fixed Message-ID whose multipart body names no boundary.
func brokenInvoiceMail(text string) []byte {
	return rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: Broken mail",
		"Date: "+quoteDate, "Message-ID: <broken-mail@example.com>", "Content-Type: multipart/mixed", "", text)
}

// unreadablePartMail returns an invoice with a fixed Message-ID whose second part, holding text, AlphOne cannot read.
func unreadablePartMail(text string) []byte {
	return rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: Invoice",
		"Date: "+quoteDate, "Message-ID: <unreadable-invoice@example.com>", "MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="files"`, "",
		"--files", "Content-Type: text/plain", "", "Your invoice is attached.",
		"--files", "Content-Type: application/x-foo bar", "", text,
		"--files--")
}

// rawCopies returns what each stored raw copy of the default workspace holds.
func (it *ingestTest) rawCopies() [][]byte {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(), "SELECT raw_file_id FROM plugin_email.messages WHERE tenant_id = $1",
		sdk.DefaultTenantID)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		it.t.Fatalf("reading the raw copies: %v", err)
	}
	var kept [][]byte
	for _, id := range ids {
		kept = append(kept, storedBytes(it.t, it.store, sdk.DefaultTenantID, id))
	}
	return kept
}

func TestIngestKeepsTwoMailsWithOneIDAndOtherContentItCannotReadWhole(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name   string
		change func(*settings)
		march  []byte
		april  []byte
		want   Outcome
	}{
		{
			name: "past the body cap", change: func(s *settings) { s.parse.MaxBodyBytes = 16 },
			march: quoteMail(nil, "Here is the invoice for March."), april: quoteMail(nil, "Here is the invoice for April."),
			want: outcomeStored,
		},
		{
			name: "too broken to read", change: func(*settings) {}, march: brokenInvoiceMail("Invoice for March."),
			april: brokenInvoiceMail("Invoice for April."), want: outcomeStoredHeadersOnly,
		},
		{
			name: "a part it cannot read", change: func(*settings) {}, march: unreadablePartMail("Invoice for March."),
			april: unreadablePartMail("Invoice for April."), want: outcomeStored,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t, c.change)
			it.ingest(it.incoming(c.march, "file:march"))

			outcome := it.ingest(it.incoming(c.april, "file:april"))

			if outcome != c.want || it.mails() != 2 {
				t.Errorf("ingest() = %q with %d mails stored, want %q and two mails", outcome, it.mails(), c.want)
			}
			kept := it.rawCopies()
			for name, raw := range map[string][]byte{"March": c.march, "April": c.april} {
				if !slices.ContainsFunc(kept, func(held []byte) bool { return bytes.Equal(held, raw) }) {
					t.Errorf("the %d raw copies hold no copy of the %s mail as fed", len(kept), name)
				}
			}
		})
	}
}

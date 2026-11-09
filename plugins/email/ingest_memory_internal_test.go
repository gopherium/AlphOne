// SPDX-License-Identifier: Elastic-2.0

//go:build !race

package email

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// largeMailBytes is the size of the largest mail a server takes, as the message package measures it.
const largeMailBytes = 34 << 20

// ingestMemoryBudget is the most one ingest of a mail may allocate in all, the budget of the message package.
const ingestMemoryBudget = 1 << 30

// droppingStore is a file store that hashes the bytes of each file it is handed and keeps none.
type droppingStore struct {
	sdk.FileStore
}

// SaveAs hashes body and answers the file a store would keep, keeping nothing.
func (droppingStore) SaveAs(_ context.Context, id uuid.UUID, contentType string, body io.Reader) (
	sdk.StoredFile, error) {
	hash := sha256.New()
	size, err := io.Copy(hash, body)
	return sdk.StoredFile{ID: id, Size: size, ContentType: contentType, SHA256: hex.EncodeToString(hash.Sum(nil))}, err
}

// filledTo answers line repeated up to largeMailBytes.
func filledTo(line string) string {
	return strings.Repeat(line, largeMailBytes/len(line))
}

func TestIngestStaysInsideTheMemoryBudget(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"an HTML body", "Content-Type: text/html\r\n\r\n" + filledTo("<p>The quote for the new wall.</p>\r\n")},
		{"an HTML body of list items", "Content-Type: text/html\r\n\r\n<ul>" + filledTo("<li>a")},
		{"an HTML body of links", "Content-Type: text/html\r\n\r\n" + filledTo(`<a href="https://example.com/">a</a>`)},
		{"a text body", "Content-Type: text/plain\r\n\r\n" + filledTo("The quote for the new wall.\r\n")},
		{
			"one attachment",
			"Content-Type: multipart/mixed; boundary=\"file\"\r\n\r\n--file\r\nContent-Type: application/pdf\r\n" +
				"Content-Disposition: attachment; filename=\"quote.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
				filledTo(strings.Repeat("QUJD", 19)+"\r\n") + "--file--\r\n",
		},
		{"one long subject", "Subject: " + filledTo("quote ") + "\r\n\r\nText.\r\n"},
		{"many recipients", "To: " + filledTo("a@example.com, ") + "b@example.com\r\n\r\nText.\r\n"},
	}

	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := newIngestTest(t)
			it.ingester.files.store = droppingStore{}
			raw := []byte(fmt.Sprintf("From: Maria Perez <maria.perez@example.com>\r\nTo: info@example.com\r\n"+
				"Message-ID: <shape-%d@example.com>\r\n", i) + c.raw)
			in := it.incoming(raw, fmt.Sprintf("file:shape-%d", i))
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)

			outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, in)

			runtime.ReadMemStats(&after)
			if err != nil || outcome != outcomeStored && outcome != outcomeStoredHeadersOnly {
				t.Fatalf("ingest() = %q, %v, want the mail stored", outcome, err)
			}
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > ingestMemoryBudget {
				t.Errorf("ingesting %d bytes allocated %d MiB, want at most %d MiB", len(raw), allocated>>20,
					ingestMemoryBudget>>20)
			}
		})
	}
}

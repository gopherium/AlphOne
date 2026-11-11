// SPDX-License-Identifier: Elastic-2.0

//go:build !race

package email

import (
	"bytes"
	"io"
	"runtime"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

// stdinFillerBytes is how far the body of the mail on standard input runs past its headers.
const stdinFillerBytes = 256 << 20

// stdinMessageCap is the message cap the line reads its mail under.
const stdinMessageCap = 1 << 20

// stdinMemoryBudget is the most one line of email:ingest may allocate for a mail far over the message cap.
const stdinMemoryBudget = 64 << 20

// repeatedLetter is an endless reader of the letter a.
type repeatedLetter struct{}

// Read fills p with the letter a.
func (repeatedLetter) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

func TestFeedMailReadsAMailFarOverTheMessageCapInsideTheMemoryBudget(t *testing.T) {
	it := newIngestTest(t, func(s *settings) { s.parse.MaxBytes = stdinMessageCap })
	stdin := io.MultiReader(bytes.NewReader(quoteMail(nil, "")), io.LimitReader(repeatedLetter{}, stdinFillerBytes))
	line := commandLine{workspace: sdk.DefaultTenantID.String(), stdin: stdin, apply: true}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	answered, err := line.run(t, it.ingester)

	runtime.ReadMemStats(&after)
	if err != nil || answered != "stored_headers_only\n" {
		t.Fatalf("feedMail() = %q, %v, want stored_headers_only", answered, err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > stdinMemoryBudget {
		t.Errorf("feeding %d MiB past a %d MiB cap allocated %d MiB, want at most %d MiB",
			stdinFillerBytes>>20, stdinMessageCap>>20, allocated>>20, stdinMemoryBudget>>20)
	}
}

// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

func TestIngestKeepsTheTextAndSkipsTheFilesOfAMailOverTheWorkspaceLimit(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.files.workspaceMaxBytes = 100 })

	outcome := it.ingest(it.incoming(invoiceMail("Invoice for March"), "file:invoice"))

	if outcome != outcomeStored {
		t.Fatalf("ingest() = %q, want %q", outcome, outcomeStored)
	}
	id := it.mailTitled("Invoice")
	row := storedRow(t, it.pool, id)
	if row.rawState != fileOverLimit || row.rawFileID != uuid.Nil || strings.TrimSpace(row.bodyText) == "" {
		t.Errorf("the raw copy is %s under %v with the text %q, want over the limit with no file and the text kept",
			row.rawState, row.rawFileID, row.bodyText)
	}
	if files := it.attachmentsOf(id); len(files) != 1 || files[0].state != fileOverLimit ||
		files[0].fileID != uuid.Nil || files[0].filename != "invoice.pdf" {
		t.Errorf("the attachments are %+v, want invoice.pdf named and over the limit with no file", files)
	}
	if types := it.store.savedTypes(); len(types) != 0 {
		t.Errorf("the store saved %q, want no file", types)
	}
	if used := usageOf(t, it.pool, sdk.DefaultTenantID); used != textBytes(t, row) {
		t.Errorf("the workspace uses %d bytes, want the text of the mail alone, %d", used, textBytes(t, row))
	}
}

func TestIngestCountsTheFilesAndTheTextOfAMailAgainstTheWorkspaceLimit(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		short int64
		state string
		saved int
	}{
		{name: "a limit the mail fills exactly", short: 0, state: fileStored, saved: 2},
		{name: "a limit one byte short of the mail", short: 1, state: fileOverLimit, saved: 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			raw := invoiceMail("Invoice for March")
			mail, err := it.ingester.readIncoming(t.Context(), it.box, it.incoming(raw, "file:invoice"))
			if err != nil {
				t.Fatalf("readIncoming() error = %v, want nil", err)
			}
			whole := int64(len(raw)) + int64(len(mail.message.Attachments[0].Content)) +
				textBytes(t, it.ingester.rowOf(mail))
			it.ingester.files.caps.workspaceMaxBytes = int(whole - c.short)

			outcome := it.ingest(it.incoming(raw, "file:invoice"))

			if outcome != outcomeStored {
				t.Fatalf("ingest() = %q, want %q", outcome, outcomeStored)
			}
			id := it.mailTitled("Invoice")
			files := it.attachmentsOf(id)
			row := storedRow(t, it.pool, id)
			if row.rawState != c.state || len(files) != 1 || files[0].state != c.state {
				t.Errorf("the raw copy is %s and the attachments are %+v, want both %s under a limit of %d for a mail of %d",
					row.rawState, files, c.state, whole-c.short, whole)
			}
			if saved := len(it.store.savedTypes()); saved != c.saved {
				t.Errorf("the store saved %d files, want %d", saved, c.saved)
			}
			counted := textBytes(t, row)
			if c.state == fileStored {
				counted = whole
			}
			if used := usageOf(t, it.pool, sdk.DefaultTenantID); row.storedBytes != counted || used != counted {
				t.Errorf("the mail added %d bytes and the workspace uses %d, want the %d the limit was checked with",
					row.storedBytes, used, counted)
			}
		})
	}
}

func TestIngestCountsNoFileOverItsCapAgainstTheWorkspaceLimit(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.files.attachmentMaxBytes = 16 })
	raw := invoiceMail("%PDF-1.4 small", strings.Repeat("x", 40))
	mail, err := it.ingester.readIncoming(t.Context(), it.box, it.incoming(raw, "file:invoice"))
	if err != nil {
		t.Fatalf("readIncoming() error = %v, want nil", err)
	}
	limit := int64(len(raw)) + int64(len(mail.message.Attachments[0].Content)) + textBytes(t, it.ingester.rowOf(mail))
	it.ingester.files.caps.workspaceMaxBytes = int(limit)

	it.ingest(it.incoming(raw, "file:invoice"))

	id := it.mailTitled("Invoice")
	row := storedRow(t, it.pool, id)
	files := it.attachmentsOf(id)
	if row.rawState != fileStored || len(files) != 2 || files[0].state != fileStored || files[1].state != fileTooLarge {
		t.Errorf("the raw copy is %s and the attachments are %+v, want the raw copy and the small file stored and the "+
			"large one too large under a limit of %d", row.rawState, files, limit)
	}
	if used := usageOf(t, it.pool, sdk.DefaultTenantID); row.storedBytes != limit || used != limit {
		t.Errorf("the mail added %d bytes and the workspace uses %d, want the limit of %d it fills", row.storedBytes,
			used, limit)
	}
}

func TestIngestHoldsNoMoreMailsInMemoryThanItsSlots(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		slots int
		mails int
	}{
		{name: "one slot by default", slots: defaultMailsInMemoryMax, mails: 3},
		{name: "two slots", slots: 2, mails: 4},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t, func(s *settings) { s.caps.mailsInMemoryMax = c.slots })
			var mu sync.Mutex
			inside, peak := 0, 0
			entered, release := make(chan struct{}, c.mails), make(chan struct{})
			it.matcher.hook = func() {
				mu.Lock()
				inside++
				peak = max(peak, inside)
				mu.Unlock()
				entered <- struct{}{}
				<-release
				mu.Lock()
				inside--
				mu.Unlock()
			}
			done := make(chan error, c.mails)
			for n := range c.mails {
				go func() {
					_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID,
						it.incoming(numberedMail(n), fmt.Sprintf("file:order-%d", n)))
					done <- err
				}()
			}
			for range c.slots {
				<-entered
			}
			time.Sleep(quietSpell)
			mu.Lock()
			held := inside
			mu.Unlock()
			close(release)
			for range c.mails {
				if err := <-done; err != nil {
					t.Errorf("ingest() error = %v, want nil", err)
				}
			}

			if held != c.slots || peak != c.slots {
				t.Errorf("%d mails were read at once and at most %d, want %d", held, peak, c.slots)
			}
			if mails := it.mails(); mails != c.mails {
				t.Errorf("the workspace holds %d mails, want %d", mails, c.mails)
			}
		})
	}
}

func TestIngestHoldsItsSlotWhileItWaitsForTheMailboxLock(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	holder := it.holdMailbox()
	first, second := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(numberedMail(1), "file:order-1"))
		first <- err
	}()
	it.awaitLockWaits(1)
	go func() {
		_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(numberedMail(2), "file:order-2"))
		second <- err
	}()
	it.awaitStep("gate", 2)
	time.Sleep(quietSpell)
	matched := len(it.matcher.calls())

	if err := holder.Rollback(t.Context()); err != nil {
		t.Fatalf("releasing the mailbox: %v", err)
	}

	if err := errors.Join(<-first, <-second); err != nil {
		t.Errorf("ingest() error = %v, want nil", err)
	}
	if matched != 1 {
		t.Errorf("%d mails were matched while the first waited for the mailbox lock, want the second held back", matched)
	}
	if mails := it.mails(); mails != 2 {
		t.Errorf("the workspace holds %d mails, want 2", mails)
	}
}

func TestIngestEndsItsWaitForASlotWithItsContext(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingester.slots <- struct{}{}
	ctx, cancel := context.WithTimeout(t.Context(), callerDeadline)
	defer cancel()

	_, err := it.ingester.ingest(ctx, sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, context.DeadlineExceeded) ||
		!strings.HasPrefix(fmt.Sprint(err), "email: wait for an ingest slot: ") {
		t.Errorf("ingest() error = %v, want the wait for a slot to end at %v", err, context.DeadlineExceeded)
	}
	if calls := it.matcher.calls(); len(calls) != 0 {
		t.Errorf("the matcher was asked %q, want the mail never read", calls)
	}
}

func TestTakeSlotEndsWithItsContextWhenNoSlotIsFree(t *testing.T) {
	t.Parallel()

	p := &ingester{slots: make(chan struct{}, 1)}
	p.slots <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	release, err := p.takeSlot(ctx)

	if !errors.Is(err, context.Canceled) || release != nil {
		t.Errorf("takeSlot() error = %v, want %v and no release", err, context.Canceled)
	}
}

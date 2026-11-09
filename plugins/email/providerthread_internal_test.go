// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"testing"
	"time"
)

func TestAMailWithinTheWindowOfTheNewestProviderThreadJoinsIt(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	day := 24 * time.Hour
	first := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice",
		"Message-ID: <invoice-march@example.com>"), arrival{providerThread: "file:conversation"})
	second := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice",
		"Message-ID: <invoice-april@example.com>"), arrival{providerThread: "file:conversation", after: 30 * day})
	if second == first {
		t.Fatal("the second invoice joined the first thread, want a new one")
	}

	third := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice question",
		"Message-ID: <invoice-question@example.com>"), arrival{providerThread: "file:conversation", after: 31 * day})

	if third != second {
		t.Errorf("a mail a day after the second invoice under its provider thread id sits in %v, want %v (first %v)",
			third, second, first)
	}
	if threads := tt.threads(); threads != 2 {
		t.Errorf("the mailbox holds %d threads, want 2", threads)
	}
}

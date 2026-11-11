// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"testing"

	"github.com/gopherium/alphone/sdk"
)

func TestIngestCountsTheAddressesACopyAddsInTheStoredBytesAndTheWorkspaceUsage(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:inbox"))
	id := it.mailTitled("Your quote")
	before := storedRow(t, it.pool, id)
	usedBefore := usageOf(t, it.pool, sdk.DefaultTenantID)

	outcome := it.ingest(it.incoming(lampMail("Cc: Ada Lovelace <ada@example.com>"), "file:archive"))

	if outcome != outcomeLinked {
		t.Fatalf("ingest() = %q, want %q", outcome, outcomeLinked)
	}
	after := storedRow(t, it.pool, id)
	grown := textBytes(t, after) - textBytes(t, before)
	if grown <= 0 {
		t.Fatalf("the participants grew by %d bytes, want the added Cc counted", grown)
	}
	if after.storedBytes != before.storedBytes+grown {
		t.Errorf("the mail keeps stored_bytes %d, want %d, the first copy's %d and the %d bytes its added Cc takes",
			after.storedBytes, before.storedBytes+grown, before.storedBytes, grown)
	}
	if used := usageOf(t, it.pool, sdk.DefaultTenantID); used != usedBefore+grown {
		t.Errorf("the workspace uses %d bytes, want %d, the first copy's %d and the %d bytes its added Cc takes",
			used, usedBefore+grown, usedBefore, grown)
	}
}

func TestWriteMailReportsTheAddressReadAndTheUsageOfALinkedCopyThatFail(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{
		"SELECT participants FROM plugin_email.messages", "INSERT INTO plugin_email.workspace_usage",
	} {
		t.Run(fragment, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			it.ingest(it.incoming(infoMail(), "file:inbox"))
			mail, row, kept := it.prepareMail(infoMail(receivedLine), "file:sent", true)
			tx := batchFailingTx{failingTx{Tx: beginTx(t, it.pool), fragment: fragment}}

			_, err := it.ingester.writeMail(t.Context(), tx, sdk.DefaultTenantID, mail, row, kept)

			if pgCode(err) != undefinedFunction {
				t.Errorf("writeMail() error = %v, want the injected failure", err)
			}
		})
	}
}

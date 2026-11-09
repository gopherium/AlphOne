// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"testing"
	"time"

	"github.com/gopherium/alphone/sdk"
)

// deliveryMail returns a mail from Maria Perez asking when the lamp arrives, with no reply ids.
func deliveryMail() []byte {
	return rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: Delivery date",
		"Date: "+quoteDate, "Message-ID: <delivery@example.com>", "Content-Type: text/plain", "",
		"When does the lamp arrive?")
}

func TestACopyBringingAProviderThreadIDLeadsLaterMailToTheStoredThread(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:inbox"))
	copied := it.incoming(lampMail(receivedLine), "file:archive")
	copied.ProviderThreadID, copied.ReceivedAt = "file:conversation", mailReceived.Add(time.Minute)
	if outcome := it.ingest(copied); outcome != outcomeDuplicate {
		t.Fatalf("the copy answered %q, want duplicate", outcome)
	}
	later := Incoming{
		MailboxID: it.box.id, Raw: bytes.NewReader(deliveryMail()), ProviderRef: "file:delivery",
		ProviderThreadID: "file:conversation", ReceivedAt: mailReceived.Add(time.Hour),
	}

	outcome := it.ingest(later)

	if outcome != outcomeStored {
		t.Fatalf("the later mail answered %q, want stored", outcome)
	}
	lamp := storedRow(t, it.pool, it.mailTitled("Your quote")).threadID
	if delivery := storedRow(t, it.pool, it.mailTitled("Delivery date")).threadID; delivery != lamp {
		t.Errorf("a later mail under the provider thread id only a copy carried sits in %v, want the copy's thread %v",
			delivery, lamp)
	}
}

func TestWriteMailReportsAFailedKeyClaimOfALinkedCopy(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(infoMail(), "file:inbox"))
	mail, row, kept := it.prepareMail(infoMail(receivedLine), "file:sent", true)
	tx := batchFailingTx{failingTx{Tx: beginTx(t, it.pool), fragment: "WITH mail AS"}}

	_, err := it.ingester.writeMail(t.Context(), tx, sdk.DefaultTenantID, mail, row, kept)

	if pgCode(err) != undefinedFunction {
		t.Errorf("writeMail() error = %v, want the injected failure", err)
	}
}

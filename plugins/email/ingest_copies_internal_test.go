// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

func TestIngestAnswersDuplicateForARedeliveryBeforeReadingIt(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	before := len(it.log.steps())

	outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if outcome != outcomeDuplicate {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeDuplicate)
	}
	if steps := it.log.steps()[before:]; !slices.Equal(steps, []string{"gate"}) {
		t.Errorf("the redelivery reached %q, want the gate alone and no read or save", steps)
	}
	if mails, events := it.mails(), it.events(); mails != 1 || len(events) != 1 {
		t.Errorf("the workspace holds %d mails and %d events, want 1 of each", mails, len(events))
	}
}

func TestIngestAnswersDuplicateForTheSameMailFromAnotherFolder(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:inbox"))

	outcome := it.ingest(it.incoming(lampMail(receivedLine), "file:archive"))

	if outcome != outcomeDuplicate {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeDuplicate)
	}
	id := it.mailTitled("Your quote")
	if refs := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.provider_refs WHERE message_id = $1",
		id); refs != 2 {
		t.Errorf("the mail has %d provider references, want 2", refs)
	}
	if types, events := it.store.savedTypes(), it.events(); len(types) != 1 || len(events) != 1 {
		t.Errorf("the copies saved %d files and published %d events, want the first copy's alone", len(types),
			len(events))
	}
	if states := it.queueStates(); len(states) != 0 {
		t.Errorf("the queue holds %q, want none", states)
	}
}

func TestIngestLinksASecondCopyNamingAnotherContact(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:inbox"))

	outcome := it.ingest(it.incoming(lampMail("Cc: Ada Lovelace <ada@example.com>"), "file:archive"))

	if outcome != outcomeLinked {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeLinked)
	}
	id := it.mailTitled("Your quote")
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com", "ada@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez and Ada", links)
	}
	addedCc := participant{Role: "cc", Name: "Ada Lovelace", Email: "ada@example.com"}
	if row := storedRow(t, it.pool, id); !slices.Contains(row.participants, addedCc) || len(row.participants) != 3 {
		t.Errorf("the participants are %+v, want the first copy's and %+v", row.participants, addedCc)
	}
	if events := it.events(); len(events) != 1 {
		t.Errorf("the copies published %d events, want the first copy's alone", len(events))
	}
}

func TestIngestTurnsTheRowOutboundForASentCopyAndAddsItsBcc(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(infoMail(receivedLine), "file:inbox"))
	sent := it.incoming(infoMail("Bcc: Ada Lovelace <ada@example.com>"), "file:sent")
	sent.SentCopy = true

	outcome := it.ingest(sent)

	if outcome != outcomeLinked {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeLinked)
	}
	id := it.mailTitled("Your quote")
	row := storedRow(t, it.pool, id)
	addedBcc := participant{Role: "bcc", Name: "Ada Lovelace", Email: "ada@example.com"}
	if row.direction != outbound || !slices.Contains(row.participants, addedBcc) {
		t.Errorf("the mail is %s with %+v, want outbound with %+v", row.direction, row.participants, addedBcc)
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com", "ada@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez and Ada", links)
	}
}

func TestIngestAnswersTheMissOfAMailFoundByItsDedupeKeyThatIsGoneUnderTheLock(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:inbox"))
	id := it.mailTitled("Your quote")
	holder := it.holdMailbox()
	ran := make(chan error, 1)
	go func() {
		_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(receivedLine), "file:archive"))
		ran <- err
	}()
	it.awaitLockWaits(1)
	if _, err := holder.Exec(t.Context(), "DELETE FROM plugin_email.messages WHERE id = $1", id); err != nil {
		t.Fatalf("deleting the stored mail by hand: %v", err)
	}
	if err := holder.Commit(t.Context()); err != nil {
		t.Fatalf("committing the delete: %v", err)
	}

	err := <-ran

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("ingest() error = %v, want the miss of the read under the lock, %v", err, pgx.ErrNoRows)
	}
	if failure := heldFailure(t, it.pool, it.box, "file:archive"); failure.reason != temporaryDatabaseError {
		t.Errorf("the failure is %+v, want a temporary one so the next run takes the full path", failure)
	}
}

func TestIngestPublishesNothingForTheCopyThatLostTheRace(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.caps.mailsInMemoryMax = 2 })
	holder := it.holdMailbox()
	outcomes, errs := make(chan Outcome, 2), make(chan error, 2)
	for _, raw := range []struct {
		mail []byte
		ref  string
	}{{lampMail(), "file:inbox"}, {lampMail(receivedLine), "file:archive"}} {
		go func() {
			outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(raw.mail, raw.ref))
			outcomes <- outcome
			errs <- err
		}()
	}
	it.awaitLockWaits(2)

	if err := holder.Rollback(t.Context()); err != nil {
		t.Fatalf("releasing the mailbox: %v", err)
	}

	got := []Outcome{<-outcomes, <-outcomes}
	if err := errors.Join(<-errs, <-errs); err != nil {
		t.Fatalf("ingest() error = %v, want nil", err)
	}
	slices.Sort(got)
	if !slices.Equal(got, []Outcome{outcomeDuplicate, outcomeStored}) {
		t.Errorf("the two copies answered %q, want one stored and one duplicate", got)
	}
	if mails, events := it.mails(), it.events(); mails != 1 || len(events) != 1 {
		t.Errorf("the workspace holds %d mails and %d events, want 1 of each", mails, len(events))
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy of the losing copy queued for deletion", states)
	}
}

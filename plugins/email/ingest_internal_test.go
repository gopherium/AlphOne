// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

func TestNewIngesterTakesItsPartsFromTheDepsAndTheSettings(t *testing.T) {
	t.Parallel()

	held, err := readSettings(sdk.Deps{})
	if err != nil {
		t.Fatalf("readSettings() error = %v, want nil", err)
	}
	held.caps.mailsInMemoryMax = 3
	matcher, publisher := &fakeMatcher{}, &steppingPublisher{}
	logger, _ := bufferLogger()

	p := newIngester(nil, sdk.Deps{Matcher: matcher, ContactMatchMax: 7, Events: publisher, Logger: logger}, held)

	if p.matcher != matcher || p.matchMax != 7 || p.publisher != publisher || p.logger != logger {
		t.Error("newIngester() left out the matcher, the match max, the publisher or the logger of the deps")
	}
	if cap(p.slots) != 3 || p.parse != held.parse || p.caps != held.caps || p.times != held.times {
		t.Errorf("newIngester() holds %d slots and other limits, want %d and the limits of the settings",
			cap(p.slots), 3)
	}
	if p.files.caps != held.files || p.files.cleanupTimeout != held.times.cleanupTimeout || p.files.logger != logger {
		t.Error("newIngester() hands the files other caps, another clean up timeout or another logger")
	}
	if p.now == nil {
		t.Error("newIngester() holds no clock, want the wall clock")
	}
}

func TestIngestStoresAMailFromAKnownContactWithItsRawCopy(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	raw := lampMail()

	outcome := it.ingest(it.incoming(raw, "file:quote"))

	if outcome != outcomeStored {
		t.Fatalf("ingest() = %q, want %q", outcome, outcomeStored)
	}
	id := it.mailTitled("Your quote")
	row := storedRow(t, it.pool, id)
	if row.direction != inbound || row.bodyState != bodyWhole || strings.TrimSpace(row.bodyText) != lampText {
		t.Errorf("the mail is %s and %s with the text %q, want inbound and whole with %q",
			row.direction, row.bodyState, row.bodyText, lampText)
	}
	if row.rawState != fileStored || row.rawSize != int64(len(raw)) || !bytes.Equal(row.rawSHA256, sum(string(raw))) {
		t.Errorf("the raw copy is %s, %d bytes, hash %x, want stored with the size and the hash of the mail",
			row.rawState, row.rawSize, row.rawSHA256)
	}
	if kept := storedBytes(t, it.store, sdk.DefaultTenantID, row.rawFileID); !bytes.Equal(kept, raw) {
		t.Errorf("the raw copy holds %q, want the mail as fed", kept)
	}
	if types := it.store.savedTypes(); !slices.Equal(types, []string{rawMailType}) {
		t.Errorf("the saved files are %q, want the raw copy as %s alone", types, rawMailType)
	}
	want := []participant{
		{Role: "from", Name: "Maria Perez", Email: "maria.perez@example.com"},
		{Role: "to", Email: "info@example.com"},
	}
	if !reflect.DeepEqual(row.participants, want) {
		t.Errorf("the participants are %+v, want %+v", row.participants, want)
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez alone", links)
	}
	if refs := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.provider_refs WHERE message_id = $1",
		id); refs != 1 {
		t.Errorf("the mail has %d provider references, want 1", refs)
	}
	if states := it.queueStates(); len(states) != 0 {
		t.Errorf("the queue holds %q, want the run's rows removed at its commit", states)
	}
	if used := usageOf(t, it.pool, sdk.DefaultTenantID); used != row.storedBytes ||
		row.storedBytes != int64(len(raw))+textBytes(t, row) {
		t.Errorf("the workspace uses %d bytes and the mail added %d, want the raw copy and the text, %d",
			used, row.storedBytes, int64(len(raw))+textBytes(t, row))
	}
	if events := it.events(); len(events) != 1 || events[0].data["id"] != id.String() {
		t.Errorf("the events are %+v, want one announcing %v", events, id)
	}
}

func TestIngestStoresTheHeadersOfAMailOverTheMessageCap(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.parse.MaxBytes = 300 })
	raw := quoteMail(nil, strings.Repeat(lampText+" ", 10))

	outcome := it.ingest(it.incoming(raw, "file:quote"))

	if outcome != outcomeStoredHeadersOnly {
		t.Fatalf("ingest() = %q, want %q", outcome, outcomeStoredHeadersOnly)
	}
	id := it.mailTitled("Your quote")
	row := storedRow(t, it.pool, id)
	if row.bodyState != bodyHeadersOnly || row.bodyText != "" || row.safeHTML != "" {
		t.Errorf("the mail is %s with the text %q and the HTML %q, want its headers alone",
			row.bodyState, row.bodyText, row.safeHTML)
	}
	if row.rawState != fileTooLarge || row.rawFileID != uuid.Nil || row.rawSize != int64(len(raw)) ||
		!bytes.Equal(row.rawSHA256, sum(string(raw))) {
		t.Errorf("the raw copy is %s under %v with %d bytes, want too large, no file and the size of the whole mail",
			row.rawState, row.rawFileID, row.rawSize)
	}
	if types := it.store.savedTypes(); len(types) != 0 {
		t.Errorf("the store saved %q, want no file", types)
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez", links)
	}
	if events := it.events(); len(events) != 0 {
		t.Errorf("the events are %+v, want none for a mail read by its headers alone", events)
	}
}

func TestIngestStoresTheHeadersAndTheRawCopyOfAMailTooBrokenToRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	raw := brokenMail("Maria Perez <maria.perez@example.com>")

	outcome := it.ingest(it.incoming(raw, "file:broken"))

	if outcome != outcomeStoredHeadersOnly {
		t.Fatalf("ingest() = %q, want %q", outcome, outcomeStoredHeadersOnly)
	}
	row := storedRow(t, it.pool, it.mailTitled("Broken mail"))
	if row.bodyState != bodyHeadersOnly || row.rawState != fileStored {
		t.Errorf("the mail is %s with its raw copy %s, want its headers alone and its raw copy stored",
			row.bodyState, row.rawState)
	}
	if kept := storedBytes(t, it.store, sdk.DefaultTenantID, row.rawFileID); !bytes.Equal(kept, raw) {
		t.Errorf("the raw copy holds %q, want the mail as fed", kept)
	}
}

func TestIngestStoresABodyOverTheBodyCapCut(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.parse.MaxBodyBytes = 16 })

	it.ingest(it.incoming(lampMail(), "file:quote"))

	row := storedRow(t, it.pool, it.mailTitled("Your quote"))
	if row.bodyState != bodyCut || row.bodyText != "Could you send m" || row.rawState != fileStored {
		t.Errorf("the mail is %s with the text %q and its raw copy %s, want cut to %q with its raw copy stored",
			row.bodyState, row.bodyText, row.rawState, "Could you send m")
	}
}

func TestIngestAnswersNoKnownContactAndLeavesNothing(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	outcome := it.ingest(it.incoming(strangerMail(), "file:offer"))

	if outcome != outcomeNoKnownContact {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeNoKnownContact)
	}
	if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the workspace holds %d rows of mail, want none", rows)
	}
	if types := it.store.savedTypes(); len(types) != 0 {
		t.Errorf("the store saved %q, want no file", types)
	}
	if batches := it.matcher.calls(); !reflect.DeepEqual(batches, [][]string{{"stranger@example.com"}}) {
		t.Errorf("the matcher was asked %q, want the stranger alone", batches)
	}
	if lines := loggedLines(t, it.logged); len(lines) != 0 {
		t.Errorf("ingest() logged %v, want nothing for a mail it read whole", lines)
	}
}

func TestIngestLogsTheMailboxAndTheReasonOfAMailReadByItsHeadersThatNamesNoKnownContact(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name   string
		change func(*settings)
		raw    []byte
		reason string
	}{
		{name: "over the message cap", change: func(s *settings) { s.parse.MaxBytes = 200 },
			raw: strangerMail(), reason: unreadableOverALimit},
		{name: "over the part limit", change: func(s *settings) { s.parse.MaxParts = 1 },
			raw: invoiceMail("Invoice for March", "Invoice for April"), reason: unreadableOverALimit},
		{name: "too broken to read", change: func(*settings) {}, raw: brokenMail("stranger@example.com"),
			reason: unreadableMalformed},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t, c.change)
			it.matcher.known[sdk.DefaultTenantID] = nil

			outcome := it.ingest(it.incoming(c.raw, "file:unknown"))

			if outcome != outcomeHeadersOnlyNoKnownContact {
				t.Errorf("ingest() = %q, want %q", outcome, outcomeHeadersOnlyNoKnownContact)
			}
			lines := loggedLines(t, it.logged)
			if len(lines) != 1 || lines[0]["mailbox"] != it.box.id.String() || lines[0]["reason"] != c.reason ||
				lines[0]["workspace"] != sdk.DefaultTenantID.String() {
				t.Errorf("ingest() logged %v, want one line naming the workspace, the mailbox and %q", lines, c.reason)
			}
			if logged := it.logged.String(); strings.Contains(logged, "@") {
				t.Errorf("ingest() logged %q, want no address", logged)
			}
			if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
				t.Errorf("the workspace holds %d rows of mail, want none", rows)
			}
		})
	}
}

func TestIngestCallsTheHostInOrderAndPublishesLast(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	it.ingest(it.incoming(invoiceMail("Invoice for March"), "file:invoice"))

	if steps := it.log.steps(); !slices.Equal(steps, []string{"gate", "match", "save", "save", "publish"}) {
		t.Errorf("the host saw %q, want the gate, the match, the raw copy, the attachment and the event", steps)
	}
}

func TestIngestPublishesOnlyAfterTheCommit(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	visible := -1
	it.publisher.hook = func(ctx context.Context, data map[string]any) {
		_ = it.pool.QueryRow(ctx, "SELECT count(*) FROM plugin_email.messages WHERE id = $1", data["id"]).
			Scan(&visible)
	}

	it.ingest(it.incoming(lampMail(), "file:quote"))

	if visible != 1 {
		t.Errorf("another session saw %d rows of the mail while its event was published, want the committed row", visible)
	}
}

func TestIngestStoresAMailThatNamesNoSender(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	outcome := it.ingest(it.incoming(rawMail("To: Maria Perez <maria.perez@example.com>", "Subject: No sender",
		"Content-Type: text/plain", "", "Who sent this?"), "file:no-sender"))

	if outcome != outcomeStored {
		t.Fatalf("ingest() = %q, want %q", outcome, outcomeStored)
	}
	row := storedRow(t, it.pool, it.mailTitled("No sender"))
	want := []participant{{Role: "to", Name: "Maria Perez", Email: "maria.perez@example.com"}}
	if !reflect.DeepEqual(row.participants, want) || !bytes.Equal(row.dedupeKey, hashParts("ref",
		[]byte("file:no-sender"))) {
		t.Errorf("the mail names %+v under the key %x, want %+v under its reference", row.participants, row.dedupeKey,
			want)
	}
}

func TestIngestJoinsAReplyToTheThreadOfItsParent(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	reply := it.incoming(rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com",
		"Subject: Re: Your quote", "Date: Mon, 02 Nov 2026 10:00:00 +0000", "Message-ID: <reply@example.com>",
		"In-Reply-To: <quote@example.com>", "Content-Type: text/plain", "", "Thank you."), "file:reply")
	reply.ReceivedAt = mailReceived.Add(time.Hour)

	it.ingest(reply)

	parent := storedRow(t, it.pool, it.mailTitled("Your quote"))
	child := storedRow(t, it.pool, it.mailTitled("Re: Your quote"))
	if child.threadID != parent.threadID {
		t.Errorf("the reply joined %v, want the thread of its parent %v", child.threadID, parent.threadID)
	}
	var subject string
	var last time.Time
	if err := it.pool.QueryRow(t.Context(), "SELECT subject, last_message_at FROM plugin_email.threads WHERE id = $1",
		parent.threadID).Scan(&subject, &last); err != nil {
		t.Fatalf("reading the thread: %v", err)
	}
	if subject != "Your quote" || !last.Equal(reply.ReceivedAt) {
		t.Errorf("the thread keeps %q up to %v, want the parent's subject up to the reply, %v", subject, last,
			reply.ReceivedAt)
	}
}

func TestIngestStoresButNeverPublishesAQuietMail(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name     string
		raw      []byte
		sentCopy bool
		backfill bool
	}{
		{name: "an auto reply", raw: lampMail("Auto-Submitted: auto-replied")},
		{name: "a Sent copy", raw: infoMail(), sentCopy: true},
		{name: "a backfill", raw: lampMail(), backfill: true},
		{name: "a forged mail from the mailbox's own address", raw: infoMail()},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			in := it.incoming(c.raw, "file:quiet")
			in.SentCopy, in.Backfill = c.sentCopy, c.backfill

			outcome := it.ingest(in)

			if outcome != outcomeStored {
				t.Errorf("ingest() = %q, want %q", outcome, outcomeStored)
			}
			if events := it.events(); len(events) != 0 {
				t.Errorf("the events are %+v, want none", events)
			}
		})
	}
}

func TestIngestKeepsAForgedMailFromTheOwnAddressInbound(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	it.ingest(it.incoming(infoMail(), "file:forged"))

	id := it.mailTitled("Your quote")
	if row := storedRow(t, it.pool, id); row.direction != inbound {
		t.Errorf("the mail is %s, want inbound whatever its From says", row.direction)
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez", links)
	}
}

func TestIngestLogsTheCountOfThePartsItCouldNotRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	it.ingest(it.incoming(rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com",
		"Subject: Two files", "MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="files"`, "",
		"--files", "Content-Type: text/plain", "", "One file reads and one does not.",
		"--files", "Content-Type: application/x-foo bar", "", "x", "--files--"), "file:two-files"))

	id := it.mailTitled("Two files")
	lines := loggedLines(t, it.logged)
	if len(lines) != 1 || lines[0]["message"] != id.String() || lines[0]["skipped_parts"] != float64(1) {
		t.Errorf("ingest() logged %v, want one line naming the mail and 1 skipped part", lines)
	}
}

func TestIngestLogsBothRowsOfAReusedMessageID(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	order := func(subject, text string) []byte {
		return rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: "+subject,
			"Date: "+quoteDate, "Message-ID: <reused-by-the-sender@example.com>", "Content-Type: text/plain", "", text)
	}
	it.ingest(it.incoming(order("First order", "Please send one brass lamp."), "file:first"))

	outcome := it.ingest(it.incoming(order("Second order", "Please send two white shades."), "file:second"))

	if outcome != outcomeStored {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeStored)
	}
	first, second := it.mailTitled("First order"), it.mailTitled("Second order")
	lines := loggedLines(t, it.logged)
	if len(lines) != 1 || lines[0]["message"] != second.String() || lines[0]["holder"] != first.String() {
		t.Errorf("ingest() logged %v, want one line naming %v and the first mail %v", lines, second, first)
	}
}

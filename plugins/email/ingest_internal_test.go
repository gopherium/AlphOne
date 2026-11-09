// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob/memblob"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/sdk"
)

// ingestNow is when the ingest tests judge and record each failure.
var ingestNow = time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)

// errGateDown is the failure of a tenant gate that cannot read its workspace.
var errGateDown = errors.New("tenant gate unreachable")

// errMatchDown is the failure of a contact match that cannot reach the contacts.
var errMatchDown = errors.New("contact match unreachable")

// errSettingsDown is the failure of a settings store that cannot be reached.
var errSettingsDown = errors.New("settings store unreachable")

// testMatchMax is the most addresses one match of the ingest tests takes.
const testMatchMax = 100

// lampText is the text of the mail asking for a lamp quote.
const lampText = "Could you send me a quote for the new lamp?"

// receivedLine is a header line a relay adds, which leaves the dedupe key of a mail as it is.
const receivedLine = "Received: from mail.example.com by mx.example.com with ESMTP"

// quietSpell is how long a test watches for a call that must not come.
const quietSpell = 200 * time.Millisecond

// callerDeadline is how long a caller with a deadline gives one ingest of a test.
const callerDeadline = 2 * time.Second

// lockWaits counts the other sessions of the same database that wait on a lock.
const lockWaits = "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()" +
	" AND pid <> pg_backend_pid() AND wait_event_type = 'Lock'"

// workspaceRows counts every row the mail of one workspace leaves in the tables of the email plugin.
const workspaceRows = `SELECT
	(SELECT count(*) FROM plugin_email.threads WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.provider_refs WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.thread_keys WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.message_contacts WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.attachments WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.failed_mail WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.workspace_usage WHERE tenant_id = $1)`

// hostCall is one call a fake of an ingest test saw, beside the workspace its context named.
type hostCall struct {
	// step names the call.
	step string
	// tenant is the workspace the call's context named, uuid.Nil when it named none.
	tenant uuid.UUID
}

// callLog lists in order the calls the fakes of one ingest test saw.
type callLog struct {
	// mu guards calls.
	mu sync.Mutex
	// calls lists every call in order.
	calls []hostCall
}

// add records step under the workspace ctx names.
func (l *callLog) add(ctx context.Context, step string) {
	tenant, _ := sdk.TenantFromContext(ctx)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, hostCall{step: step, tenant: tenant})
}

// all returns every call so far.
func (l *callLog) all() []hostCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.calls)
}

// steps returns the step of every call so far.
func (l *callLog) steps() []string {
	var steps []string
	for _, call := range l.all() {
		steps = append(steps, call.step)
	}
	return steps
}

// fakeGate answers the same for every workspace whether it takes mail, running hook ahead of each answer.
type fakeGate struct {
	// log records each call.
	log *callLog
	// closed reports a workspace that takes no mail.
	closed bool
	// err replaces the answer when set.
	err error
	// hook runs ahead of each answer when set.
	hook func()
}

// AcceptsMachineTraffic records the call, runs hook and answers whether the workspace is open.
func (g *fakeGate) AcceptsMachineTraffic(ctx context.Context, _ uuid.UUID) (bool, error) {
	g.log.add(ctx, "gate")
	if g.hook != nil {
		g.hook()
	}
	return !g.closed, g.err
}

// fakeMatcher answers the contacts it knows in the workspace a call's context names, recording every batch.
type fakeMatcher struct {
	// log records each call.
	log *callLog
	// known maps each workspace to the contact each email it knows names.
	known map[uuid.UUID]map[string]uuid.UUID
	// err replaces the answer when set.
	err error
	// hook runs ahead of each answer when set.
	hook func()
	// mu guards batches and channels.
	mu sync.Mutex
	// batches lists the identifiers of each call in order.
	batches [][]string
	// channels lists the channel of each call in order.
	channels []sdk.Channel
}

// MatchIdentities records the batch, runs hook and answers each identifier the workspace of ctx knows.
func (m *fakeMatcher) MatchIdentities(
	ctx context.Context, channel sdk.Channel, identifiers []string,
) (map[string]sdk.Contact, error) {
	m.log.add(ctx, "match")
	m.mu.Lock()
	m.batches = append(m.batches, slices.Clone(identifiers))
	m.channels = append(m.channels, channel)
	m.mu.Unlock()
	if m.hook != nil {
		m.hook()
	}
	if m.err != nil {
		return nil, m.err
	}
	matched := map[string]sdk.Contact{}
	for _, identifier := range identifiers {
		if contact, found := m.known[sdk.TenantOrDefault(ctx)][identifier]; found {
			matched[identifier] = sdk.Contact{ID: contact}
		}
	}
	return matched, nil
}

// calls returns the batches every call so far took.
func (m *fakeMatcher) calls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.batches)
}

// savingStore hands every call to the host's file store, recording each SaveAs and its type and running hook first.
type savingStore struct {
	sdk.FileStore
	// log records each SaveAs.
	log *callLog
	// hook runs ahead of each SaveAs when set, and its error replaces the save.
	hook func(ctx context.Context, id uuid.UUID) error
	// mu guards types.
	mu sync.Mutex
	// types lists the media type of each SaveAs in order.
	types []string
}

// SaveAs records the call, runs hook and saves body through the host's store unless hook fails.
func (s *savingStore) SaveAs(ctx context.Context, id uuid.UUID, contentType string, body io.Reader) (
	sdk.StoredFile, error) {
	s.log.add(ctx, "save")
	s.mu.Lock()
	s.types = append(s.types, contentType)
	s.mu.Unlock()
	if s.hook != nil {
		if err := s.hook(ctx, id); err != nil {
			return sdk.StoredFile{}, err
		}
	}
	return s.FileStore.SaveAs(ctx, id, contentType, body)
}

// savedTypes returns the media type of every SaveAs so far.
func (s *savingStore) savedTypes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.types)
}

// steppingPublisher records every event it is handed, running hook first.
type steppingPublisher struct {
	// log records each event.
	log *callLog
	// hook runs ahead of recording each event when set.
	hook func(ctx context.Context, data map[string]any)
	// mu guards events.
	mu sync.Mutex
	// events lists the events in the order they came.
	events []published
}

// Publish records the call, runs hook and records the event beside the workspace its context names.
func (p *steppingPublisher) Publish(ctx context.Context, name string, data map[string]any) {
	p.log.add(ctx, "publish")
	if p.hook != nil {
		p.hook(ctx, data)
	}
	tenant, _ := sdk.TenantFromContext(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, published{tenant: tenant, name: name, data: data})
}

// failingSettings is a settings store whose every lookup fails.
type failingSettings struct {
	sdk.SettingStore
}

// Lookup answers errSettingsDown.
func (failingSettings) Lookup(context.Context, string) (string, bool, error) {
	return "", false, errSettingsDown
}

// batchFailingTx is a failingTx that also sends injectedFailure in place of a batch holding its fragment.
type batchFailingTx struct {
	failingTx
}

// SendBatch sends batch, or a batch of injectedFailure alone when a statement of batch holds the fragment.
func (f batchFailingTx) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	for _, queued := range batch.QueuedQueries {
		if strings.Contains(queued.SQL, f.fragment) {
			failing := &pgx.Batch{}
			failing.Queue(injectedFailure)
			return f.Tx.SendBatch(ctx, failing)
		}
	}
	return f.Tx.SendBatch(ctx, batch)
}

// ingestTest is one ingest test: its database, its mailbox, its ingester and the fakes the ingester calls.
type ingestTest struct {
	t         *testing.T
	pool      *pgxpool.Pool
	box       mailbox
	ingester  *ingester
	log       *callLog
	gate      *fakeGate
	matcher   *fakeMatcher
	store     *savingStore
	publisher *steppingPublisher
	logged    *bytes.Buffer
	contacts  map[string]uuid.UUID
}

// newIngestTest returns an ingest test whose default workspace holds the mailbox and knows every known email.
func newIngestTest(t *testing.T, change ...func(*settings)) *ingestTest {
	t.Helper()
	held, err := readSettings(sdk.Deps{})
	if err != nil {
		t.Fatalf("readSettings() error = %v, want nil", err)
	}
	for _, apply := range change {
		apply(&held)
	}
	pool := newMigratedPool(t)
	log := &callLog{}
	logger, written := bufferLogger()
	it := &ingestTest{
		t: t, pool: pool, box: storeMailbox(t, pool, sdk.DefaultTenantID), log: log,
		gate:      &fakeGate{log: log},
		matcher:   &fakeMatcher{log: log, known: map[uuid.UUID]map[string]uuid.UUID{}},
		store:     &savingStore{FileStore: storage.NewFileStore(memblob.OpenBucket(nil), "email", fileCap), log: log},
		publisher: &steppingPublisher{log: log},
		logged:    written,
	}
	it.contacts = it.knowContacts(sdk.DefaultTenantID)
	it.ingester = newIngester(pool, sdk.Deps{
		Matcher: it.matcher, ContactMatchMax: testMatchMax, Events: it.publisher, Logger: logger,
	}, held)
	it.ingester.gate = it.gate
	it.ingester.files.store = it.store
	it.ingester.files.settings = postgres.NewSettingStore(pool, nil).ForPlugin("email")
	it.ingester.now = func() time.Time { return ingestNow }
	return it
}

// knowContacts stores a contact for each known email in the workspace tenant and has the matcher answer them there.
func (it *ingestTest) knowContacts(tenant uuid.UUID) map[string]uuid.UUID {
	it.t.Helper()
	known := map[string]uuid.UUID{}
	for _, email := range knownEmails {
		known[email] = storeContact(it.t, it.pool, tenant, email)
	}
	it.matcher.known[tenant] = known
	return known
}

// incoming returns raw as a mail of the test's mailbox under ref, received at mailReceived.
func (it *ingestTest) incoming(raw []byte, ref string) Incoming {
	return Incoming{MailboxID: it.box.id, Raw: bytes.NewReader(raw), ProviderRef: ref, ReceivedAt: mailReceived}
}

// ingest ingests in for the default workspace and fails the test when it answers an error.
func (it *ingestTest) ingest(in Incoming) Outcome {
	it.t.Helper()
	outcome, err := it.ingester.ingest(it.t.Context(), sdk.DefaultTenantID, in)
	if err != nil {
		it.t.Fatalf("ingest() error = %v, want nil", err)
	}
	return outcome
}

// mailTitled returns the id of the one stored mail with subject, failing the test unless exactly one holds it.
func (it *ingestTest) mailTitled(subject string) uuid.UUID {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(), "SELECT id FROM plugin_email.messages WHERE subject = $1", subject)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil || len(ids) != 1 {
		it.t.Fatalf("the mails titled %q are %v, %v, want exactly one", subject, ids, err)
	}
	return ids[0]
}

// mails counts the stored mails of the default workspace.
func (it *ingestTest) mails() int {
	it.t.Helper()
	return countRows(it.t, it.pool, "SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1",
		sdk.DefaultTenantID)
}

// rows counts every row the mail of the workspace tenant leaves.
func (it *ingestTest) rows(tenant uuid.UUID) int {
	it.t.Helper()
	return countRows(it.t, it.pool, workspaceRows, tenant)
}

// links returns the contacts the mail id is linked to, in the order of their ids.
func (it *ingestTest) links(id uuid.UUID) []uuid.UUID {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(),
		"SELECT contact_id FROM plugin_email.message_contacts WHERE message_id = $1 ORDER BY contact_id", id)
	contacts, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		it.t.Fatalf("reading the links of %v: %v", id, err)
	}
	return contacts
}

// knownContacts returns the contacts of the default workspace each email names, in the order of their ids.
func (it *ingestTest) knownContacts(emails ...string) []uuid.UUID {
	contacts := make([]uuid.UUID, 0, len(emails))
	for _, email := range emails {
		contacts = append(contacts, it.contacts[email])
	}
	return slices.SortedFunc(slices.Values(contacts), func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
}

// attachmentsOf returns the attachment rows of the mail id in position order.
func (it *ingestTest) attachmentsOf(id uuid.UUID) []attachmentRow {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(), `SELECT position, filename, content_type, size, sha256, inline,
		content_id, file_id, state FROM plugin_email.attachments WHERE message_id = $1 ORDER BY position`, id)
	held, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (attachmentRow, error) {
		var file attachmentRow
		var fileID *uuid.UUID
		err := row.Scan(&file.position, &file.filename, &file.contentType, &file.size, &file.sha256, &file.inline,
			&file.contentID, &fileID, &file.state)
		if fileID != nil {
			file.fileID = *fileID
		}
		return file, err
	})
	if err != nil {
		it.t.Fatalf("reading the attachments of %v: %v", id, err)
	}
	return held
}

// events returns every event the publisher was handed.
func (it *ingestTest) events() []published {
	it.publisher.mu.Lock()
	defer it.publisher.mu.Unlock()
	return slices.Clone(it.publisher.events)
}

// queueStates returns the state of every queue row of the default workspace, in order.
func (it *ingestTest) queueStates() []string {
	it.t.Helper()
	rows, _ := it.pool.Query(it.t.Context(),
		"SELECT state FROM plugin_email.pending_files WHERE tenant_id = $1 ORDER BY state", sdk.DefaultTenantID)
	states, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		it.t.Fatalf("reading the queue: %v", err)
	}
	return states
}

// exec runs each statement on the test's database in order.
func (it *ingestTest) exec(statements ...string) {
	it.t.Helper()
	for _, statement := range statements {
		if _, err := it.pool.Exec(it.t.Context(), statement); err != nil {
			it.t.Fatalf("running %q: %v", statement, err)
		}
	}
}

// refuseNewMail makes every new mail row fail with the SQLSTATE code, its message naming an address.
func (it *ingestTest) refuseNewMail(code string) {
	it.t.Helper()
	it.exec(`CREATE FUNCTION refuse_new_mail() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'refused the mail of maria.perez@example.com' USING ERRCODE = '`+code+`'; END $$`,
		`CREATE TRIGGER refuse_new_mail BEFORE INSERT ON plugin_email.messages
		FOR EACH ROW EXECUTE FUNCTION refuse_new_mail()`)
}

// holdMailbox locks the row of the test's mailbox in a transaction of its own, rolled back at cleanup.
func (it *ingestTest) holdMailbox() pgx.Tx {
	it.t.Helper()
	tx := beginTx(it.t, it.pool)
	if _, err := tx.Exec(it.t.Context(), "SELECT id FROM plugin_email.mailboxes WHERE id = $1 FOR UPDATE",
		it.box.id); err != nil {
		it.t.Fatalf("locking the mailbox: %v", err)
	}
	return tx
}

// awaitLockWaits returns once count other sessions of the test's database wait on a lock.
func (it *ingestTest) awaitLockWaits(count int) {
	it.t.Helper()
	started := time.Now()
	for {
		var waiting int
		if err := it.pool.QueryRow(it.t.Context(), lockWaits).Scan(&waiting); err != nil {
			it.t.Fatalf("counting the sessions that wait on a lock: %v", err)
		}
		if waiting >= count {
			return
		}
		if time.Since(started) > rivalBudget {
			it.t.Fatalf("%d sessions wait on a lock after %v, want %d", waiting, rivalBudget, count)
		}
		time.Sleep(rivalPoll)
	}
}

// awaitStep returns once the fakes saw step count times.
func (it *ingestTest) awaitStep(step string, count int) {
	it.t.Helper()
	started := time.Now()
	for {
		seen := 0
		for _, called := range it.log.steps() {
			if called == step {
				seen++
			}
		}
		if seen >= count {
			return
		}
		if time.Since(started) > rivalBudget {
			it.t.Fatalf("the fakes saw %d %s calls after %v, want %d", seen, step, rivalBudget, count)
		}
		time.Sleep(rivalPoll)
	}
}

// prepareMail reads raw under ref and keeps its files as ingest does before its transaction.
func (it *ingestTest) prepareMail(raw []byte, ref string, sentCopy bool) (parsedMail, messageRow, []keptFile) {
	it.t.Helper()
	in := it.incoming(raw, ref)
	in.SentCopy = sentCopy
	mail, err := it.ingester.readIncoming(it.t.Context(), it.box, in)
	if err != nil {
		it.t.Fatalf("readIncoming() error = %v, want nil", err)
	}
	kept, err := it.ingester.files.keepFiles(it.t.Context(), sdk.DefaultTenantID,
		it.ingester.files.filesOf(mail.message, true), fileStored)
	if err != nil {
		it.t.Fatalf("keepFiles() error = %v, want nil", err)
	}
	return mail, withFiles(it.ingester.rowOf(mail), kept), kept
}

// sweep runs one sweep of the queue over the test's database and file store.
func (it *ingestTest) sweep() {
	queueSweeper := &sweeper{pool: it.pool, store: it.store, logger: slog.New(slog.DiscardHandler),
		pace: sweepSettings{interval: time.Hour, batch: 10, budget: time.Minute, orphanAge: orphanAge}}
	queueSweeper.sweep(it.t.Context())
}

// wantFilesKept fails the test unless the store keeps every byte of each file of kept.
func (it *ingestTest) wantFilesKept(kept []keptFile) {
	it.t.Helper()
	for _, file := range kept {
		if held := storedBytes(it.t, it.store, sdk.DefaultTenantID, file.fileID()); len(held) != int(file.size) {
			it.t.Errorf("the store keeps %d bytes of %v, want its %d", len(held), file.fileID(), file.size)
		}
	}
}

// lampMail returns the mail of Maria Perez asking for a lamp quote, with header lines put before its own.
func lampMail(before ...string) []byte {
	return quoteMail(before, lampText)
}

// strangerMail returns a plain text offer from an address no contact holds, with header lines put before its own.
func strangerMail(before ...string) []byte {
	return rawMail(append(before,
		"From: stranger@example.com",
		"To: info@example.com",
		"Subject: A special offer",
		"Date: "+quoteDate,
		"Message-ID: <offer@example.com>",
		"Content-Type: text/plain",
		"",
		"Our whole price list follows below, with every lamp, every shade and every bulb we sell this season.",
	)...)
}

// brokenMail returns a mail from email whose multipart body names no boundary.
func brokenMail(email string) []byte {
	return rawMail("From: "+email, "To: info@example.com", "Subject: Broken mail", "Date: "+quoteDate,
		"Message-ID: <broken-mail@example.com>", "Content-Type: multipart/mixed", "", "The boundary is missing.")
}

// numberedMail returns a plain text mail from Maria Perez whose id and text carry n.
func numberedMail(n int) []byte {
	return rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com",
		fmt.Sprintf("Subject: Order %d", n), "Date: "+quoteDate, fmt.Sprintf("Message-ID: <order-%d@example.com>", n),
		"Content-Type: text/plain", "", fmt.Sprintf("Please send %d lamps.", n))
}

// infoMail returns the reply of the mailbox to Maria Perez, with header lines put before its own.
func infoMail(before ...string) []byte {
	return rawMail(append(before,
		"From: Info <info@example.com>",
		"To: Maria Perez <maria.perez@example.com>",
		"Subject: Your quote",
		"Date: "+quoteDate,
		"Message-ID: <your-quote@example.com>",
		"Content-Type: text/plain",
		"",
		"The quote for the new lamp is ready.",
	)...)
}

// textBytes counts the text row keeps: its body, its safe HTML, its snippet and its participants.
func textBytes(t *testing.T, row messageRow) int64 {
	t.Helper()
	participants, err := json.Marshal(row.participants)
	if err != nil {
		t.Fatalf("encoding the participants: %v", err)
	}
	return int64(len(row.bodyText) + len(row.safeHTML) + len(row.snippet) + len(participants))
}

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

func TestIngestAnswersWorkspaceClosedAndReadsNothing(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.gate.closed = true

	outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if outcome != outcomeWorkspaceClosed {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeWorkspaceClosed)
	}
	if steps := it.log.steps(); !slices.Equal(steps, []string{"gate"}) {
		t.Errorf("the host saw %q, want the gate alone", steps)
	}
	if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the workspace holds %d rows of mail, want none", rows)
	}
}

func TestIngestReportsAGateThatFails(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.gate.err = errGateDown

	outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errGateDown) || outcome != "" {
		t.Errorf("ingest() = %q, %v, want no outcome and %v", outcome, err, errGateDown)
	}
}

func TestIngestReportsAMailboxThatDoesNotExist(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	in := it.incoming(lampMail(), "file:quote")
	in.MailboxID = uuid.Must(uuid.NewV7())

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, in)

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("ingest() error = %v, want %v", err, pgx.ErrNoRows)
	}
}

func TestIngestNeedsTheTimeTheMailboxReceivedTheMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	in := it.incoming(lampMail(), "file:quote")
	in.ReceivedAt = time.Time{}

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, in)

	if !errors.Is(err, errNoReceivedTime) {
		t.Errorf("ingest() error = %v, want %v", err, errNoReceivedTime)
	}
	if steps := it.log.steps(); len(steps) != 0 {
		t.Errorf("the host saw %q, want nothing asked", steps)
	}
}

func TestIngestReportsARawMailItCannotRead(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		raw  io.Reader
	}{
		{name: "at once", raw: failingReader{}},
		{name: "past the message cap", raw: io.MultiReader(bytes.NewReader(lampMail()), failingReader{})},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t, func(s *settings) { s.parse.MaxBytes = 100 })
			in := it.incoming(nil, "file:quote")
			in.Raw = c.raw

			_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, in)

			if !errors.Is(err, errEntropy) {
				t.Errorf("ingest() error = %v, want %v", err, errEntropy)
			}
			if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
				t.Errorf("the workspace holds %d rows of mail, want none for a read the caller retries", rows)
			}
		})
	}
}

func TestIngestReportsAMatchThatFails(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.matcher.err = errMatchDown

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errMatchDown) {
		t.Errorf("ingest() error = %v, want %v", err, errMatchDown)
	}
	if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the workspace holds %d rows of mail, want none", rows)
	}
}

func TestIngestReportsAWorkspaceLimitItCannotRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingester.files.settings = failingSettings{}

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errSettingsDown) {
		t.Errorf("ingest() error = %v, want %v", err, errSettingsDown)
	}
	if rows := it.rows(sdk.DefaultTenantID); rows != 0 || len(it.store.savedTypes()) != 0 {
		t.Errorf("the workspace holds %d rows of mail and %d saved files, want none", rows, len(it.store.savedTypes()))
	}
}

func TestIngestReportsAStoredMailItCannotLookUp(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	it.matcher.hook = cancel

	_, err := it.ingester.ingest(ctx, sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, context.Canceled) {
		t.Errorf("ingest() error = %v, want %v", err, context.Canceled)
	}
}

func TestIngestReportsAFailureItCannotRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.exec("DROP TABLE plugin_email.failed_mail")

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if pgCode(err) != "42P01" {
		t.Errorf("ingest() error = %v, want the missing failed_mail table", err)
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

func TestIngestStoresASentCopyAsOutboundOnItsRecipientsAndBcc(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	sent := it.incoming(infoMail("Bcc: Ada Lovelace <ada@example.com>"), "file:sent")
	sent.SentCopy = true

	outcome := it.ingest(sent)

	if outcome != outcomeStored {
		t.Fatalf("ingest() = %q, want %q", outcome, outcomeStored)
	}
	id := it.mailTitled("Your quote")
	if row := storedRow(t, it.pool, id); row.direction != outbound {
		t.Errorf("the mail is %s, want outbound", row.direction)
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com", "ada@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez and Ada", links)
	}
}

func TestIngestMatchesNoBccOfAnInboundMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	outcome := it.ingest(it.incoming(strangerMail("Bcc: Ada Lovelace <ada@example.com>"), "file:offer"))

	if outcome != outcomeNoKnownContact {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeNoKnownContact)
	}
	if batches := it.matcher.calls(); !reflect.DeepEqual(batches, [][]string{{"stranger@example.com"}}) {
		t.Errorf("the matcher was asked %q, want the stranger alone", batches)
	}
}

func TestIngestMatchesTheAddressesInBatchesOfTheMatchMax(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingester.matchMax = 2

	it.ingest(it.incoming(lampMail("Cc: a@example.com, b@example.com, c@example.com, d@example.com"), "file:quote"))

	want := [][]string{{"maria.perez@example.com", "a@example.com"}, {"b@example.com", "c@example.com"},
		{"d@example.com"}}
	if batches := it.matcher.calls(); !reflect.DeepEqual(batches, want) {
		t.Errorf("the matcher was asked %q, want %q", batches, want)
	}
	if channels := it.matcher.channels; !slices.Equal(channels, []sdk.Channel{"email", "email", "email"}) {
		t.Errorf("the matcher was asked on %q, want the email channel every time", channels)
	}
}

func TestIngestStopsMatchingAtTheRecipientCap(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.caps.recipientsMax = 3 })

	it.ingest(it.incoming(rawMail("Cc: a@example.com, ada@example.com", "From: Maria Perez <maria.perez@example.com>",
		"To: info@example.com", "Subject: Your quote", "Content-Type: text/plain", "", lampText), "file:quote"))

	if batches := it.matcher.calls(); !reflect.DeepEqual(batches, [][]string{{"maria.perez@example.com",
		"a@example.com"}}) {
		t.Errorf("the matcher was asked %q, want the first three addresses less the mailbox", batches)
	}
	id := it.mailTitled("Your quote")
	if row := storedRow(t, it.pool, id); len(row.participants) != 3 {
		t.Errorf("the mail names %d participants, want 3", len(row.participants))
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez alone", links)
	}
}

func TestIngestCutsTheSubjectTheNamesAndTheParticipantsAtTheirCaps(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) {
		s.caps.subjectMaxBytes, s.caps.nameMaxBytes, s.caps.recipientsMax = 4, 5, 2
	})

	it.ingest(it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: Front Desk <info@example.com>", "Cc: ada@example.com",
		"Subject: =?UTF-8?Q?Caf=C3=A9_lamp?=", "Date: "+quoteDate, "Message-ID: <cafe@example.com>",
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="files"`, "",
		"--files", "Content-Type: text/plain", "", "The quote is attached.",
		"--files", "Content-Type: application/pdf", `Content-Disposition: attachment; filename="quotation.pdf"`, "",
		"%PDF-1.4", "--files--"), "file:cafe"))

	id := it.mailTitled("Caf")
	row := storedRow(t, it.pool, id)
	want := []participant{
		{Role: "from", Name: "Maria", Email: "maria.perez@example.com"},
		{Role: "to", Name: "Front", Email: "info@example.com"},
	}
	if !reflect.DeepEqual(row.participants, want) {
		t.Errorf("the participants are %+v, want %+v", row.participants, want)
	}
	if files := it.attachmentsOf(id); len(files) != 1 || files[0].filename != "quota" {
		t.Errorf("the attachments are %+v, want quotation.pdf cut to quota", files)
	}
	var subject string
	if err := it.pool.QueryRow(t.Context(), "SELECT subject FROM plugin_email.threads WHERE id = $1",
		row.threadID).Scan(&subject); err != nil || subject != "Caf" {
		t.Errorf("the thread keeps the subject %q, %v, want Caf", subject, err)
	}
	if links := it.links(id); !slices.Equal(links, it.knownContacts("maria.perez@example.com")) {
		t.Errorf("the mail is linked to %v, want Maria Perez alone, Ada cut with the participants", links)
	}
}

func TestIngestKeepsTheContentIDACidSourceDecodesTo(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	it.ingest(it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: The wall",
		"MIME-Version: 1.0", `Content-Type: multipart/related; boundary="related"`, "",
		"--related", "Content-Type: text/html", "", `<p>The wall</p><img src="cid:a%2Bb@example.com">`,
		"--related", "Content-Type: image/png", "Content-ID: <a+b@example.com>", "Content-Transfer-Encoding: base64",
		"", "iVBORw0KGgo=", "--related--"), "file:wall"))

	id := it.mailTitled("The wall")
	row := storedRow(t, it.pool, id)
	if !strings.Contains(row.safeHTML, `src="cid:a%2Bb@example.com"`) {
		t.Errorf("the safe HTML is %q, want the cid source kept as sent", row.safeHTML)
	}
	files := it.attachmentsOf(id)
	decoded, err := url.PathUnescape("a%2Bb@example.com")
	if err != nil || len(files) != 1 || !files[0].inline || files[0].contentID != decoded {
		t.Errorf("the attachments are %+v, want one inline image whose content id is %q", files, decoded)
	}
}

func TestIngestKeepsNoSnippetForAnEncryptedMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	it.ingest(it.incoming(rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com",
		"Subject: Sealed", "Content-Type: text/plain", "", "-----BEGIN PGP MESSAGE-----", "hQEMA0123456789",
		"-----END PGP MESSAGE-----"), "file:sealed"))

	row := storedRow(t, it.pool, it.mailTitled("Sealed"))
	if !row.encrypted || row.snippet != "" || !strings.Contains(row.bodyText, "BEGIN PGP MESSAGE") {
		t.Errorf("the mail is encrypted %t with the snippet %q, want encrypted with no snippet and its text kept",
			row.encrypted, row.snippet)
	}
	if events := it.events(); len(events) != 1 || events[0].data["text"] != "" {
		t.Errorf("the events are %+v, want one with an empty text", events)
	}
}

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

func TestIngestCutsAContentIDAtTheNameCap(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	long := strings.Repeat("x", 200<<10)

	it.ingest(it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: Long id",
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="files"`, "",
		"--files", "Content-Type: text/plain", "", "The quote is attached.",
		"--files", "Content-Type: application/pdf", "Content-ID: <"+long+">",
		`Content-Disposition: attachment; filename="quote.pdf"`, "", "%PDF-1.4", "--files--"), "file:long-id"))

	files := it.attachmentsOf(it.mailTitled("Long id"))
	if len(files) != 1 {
		t.Fatalf("the mail keeps %d attachments, want quote.pdf alone", len(files))
	}
	if files[0].contentID != long[:defaultNameMaxBytes] {
		t.Errorf("the attachment keeps a content id of %d bytes, want it cut to %d", len(files[0].contentID),
			defaultNameMaxBytes)
	}
}

func TestIngestStoresEachAttachmentWithTheFileKeptForIt(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t, func(s *settings) { s.files.attachmentMaxBytes = 16 })
	large := strings.Repeat("x", 40)

	it.ingest(it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: Three files",
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="outer"`, "",
		"--outer", `Content-Type: multipart/related; boundary="inner"`, "",
		"--inner", "Content-Type: text/html", "", `<p>The wall</p><img src="cid:wall@example.com">`,
		"--inner", "Content-Type: image/png", "Content-ID: <wall@example.com>", "Content-Transfer-Encoding: base64",
		"", "iVBORw0KGgo=", "--inner--",
		"--outer", "Content-Type: application/pdf", `Content-Disposition: attachment; filename="quote.pdf"`, "",
		"%PDF-1.4 quote",
		"--outer", "Content-Type: application/octet-stream", `Content-Disposition: attachment; filename="big.bin"`, "",
		large, "--outer--"), "file:three"))

	files := it.attachmentsOf(it.mailTitled("Three files"))
	if len(files) != 3 || files[0].fileID == uuid.Nil || files[1].fileID != uuid.Nil || files[2].fileID == uuid.Nil {
		t.Fatalf("the attachments are %+v, want a file for quote.pdf and the image and none for big.bin", files)
	}
	for i := range files {
		files[i].fileID = uuid.Nil
	}
	png := string([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	want := []attachmentRow{
		{position: 0, filename: "quote.pdf", contentType: "application/pdf", size: 14, sha256: sum("%PDF-1.4 quote"),
			state: fileStored},
		{position: 1, filename: "big.bin", contentType: octetStream, size: 40, sha256: sum(large), state: fileTooLarge},
		{position: 2, contentType: "image/png", size: 8, sha256: sum(png), inline: true, contentID: "wall@example.com",
			state: fileStored},
	}
	if !reflect.DeepEqual(files, want) {
		t.Errorf("the attachments are %+v, want %+v", files, want)
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

func TestIngestRollsBackAndQueuesEveryFileWhenTheSweepTookThem(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.store.hook = func(ctx context.Context, id uuid.UUID) error {
		_, err := it.pool.Exec(ctx, "UPDATE plugin_email.pending_files SET state = 'deleting' WHERE file_id = $1", id)
		return err
	}

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errFilesSwept) {
		t.Errorf("ingest() error = %v, want %v", err, errFilesSwept)
	}
	if mails := it.mails(); mails != 0 {
		t.Errorf("the workspace holds %d mails, want the commit rolled back", mails)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
	if failure := heldFailure(t, it.pool, it.box, "file:quote"); failure.reason != temporaryDatabaseError {
		t.Errorf("the failure is %+v, want a temporary one", failure)
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

func TestIngestSetsAsideAMailTheDatabaseRefusesForGood(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.refuseNewMail("22023")

	outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if outcome != outcomeFailed {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeFailed)
	}
	failure := heldFailure(t, it.pool, it.box, "file:quote")
	if failure.reason != permanentDatabaseError || failure.attempts != 1 || !failure.firstFailedAt.Equal(ingestNow) {
		t.Errorf("the failure is %+v, want one permanent attempt at %v", failure, ingestNow)
	}
	var held string
	if err := it.pool.QueryRow(t.Context(), "SELECT to_jsonb(f)::text FROM plugin_email.failed_mail f").
		Scan(&held); err != nil || strings.Contains(held, "@") || strings.Contains(held, "refused") {
		t.Errorf("the failure row holds %s, %v, want no address and no error text", held, err)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
	if mails, events := it.mails(), it.events(); mails != 0 || len(events) != 0 {
		t.Errorf("the workspace holds %d mails and %d events, want none", mails, len(events))
	}
}

func TestIngestSetsAsideAMailOnAnOutageAndAnswersItsError(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.refuseNewMail("40001")

	outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if pgCode(err) != "40001" || outcome != "" {
		t.Errorf("ingest() = %q, %v, want no outcome and the error the caller retries", outcome, err)
	}
	if failure := heldFailure(t, it.pool, it.box, "file:quote"); failure.reason != temporaryDatabaseError {
		t.Errorf("the failure is %+v, want a temporary one", failure)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
}

func TestIngestSetsAsideAMailWhoseFileTheStoreCannotSave(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.store.hook = func(context.Context, uuid.UUID) error { return errStoreDown }

	_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if !errors.Is(err, errFileStore) || !errors.Is(err, errStoreDown) {
		t.Errorf("ingest() error = %v, want %v and %v", err, errFileStore, errStoreDown)
	}
	if failure := heldFailure(t, it.pool, it.box, "file:quote"); failure.reason != fileStoreError {
		t.Errorf("the failure is %+v, want a file store one", failure)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
}

func TestIngestAnswersBothErrorsWhenItCannotSetTheMailAside(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.refuseNewMail("22023")
	it.exec("ALTER TABLE plugin_email.failed_mail ADD CONSTRAINT refuse_failures CHECK (false) NOT VALID")

	outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if outcome != "" || pgCode(err) != "22023" || !strings.Contains(fmt.Sprint(err), "SQLSTATE 23514") {
		t.Errorf("ingest() = %q, %v, want no outcome and both the refused mail and the refused failure", outcome, err)
	}
}

func TestIngestSetsAsideAMailWhoseDeadlinePassesBeforeItsCommit(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	holder := beginTx(t, it.pool)
	if _, err := holder.Exec(t.Context(), "SELECT id FROM plugin_email.mailboxes WHERE id = $1 FOR NO KEY UPDATE",
		it.box.id); err != nil {
		t.Fatalf("locking the mailbox: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), callerDeadline)
	defer cancel()
	ran := make(chan error, 1)
	go func() {
		_, err := it.ingester.ingest(ctx, sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))
		ran <- err
	}()
	it.awaitLockWaits(1)

	err := <-ran

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ingest() error = %v, want the deadline of its caller", err)
	}
	failure := heldFailure(t, it.pool, it.box, "file:quote")
	if failure.reason != temporaryDatabaseError || failure.attempts != 1 || !failure.firstFailedAt.Equal(ingestNow) {
		t.Errorf("the failure is %+v, want one temporary attempt at %v", failure, ingestNow)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting"}) {
		t.Errorf("the queue holds %q, want the raw copy queued for deletion", states)
	}
}

func TestIngestTriesAgainAMailWhoseOnlyRunWasCancelled(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	holder := beginTx(t, it.pool)
	if _, err := holder.Exec(t.Context(), "SELECT id FROM plugin_email.mailboxes WHERE id = $1 FOR NO KEY UPDATE",
		it.box.id); err != nil {
		t.Fatalf("locking the mailbox: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	ran := make(chan error, 1)
	go func() {
		_, err := it.ingester.ingest(ctx, sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))
		ran <- err
	}()
	it.awaitLockWaits(1)
	cancel()
	if err := <-ran; !errors.Is(err, context.Canceled) {
		t.Fatalf("ingest() error = %v, want the cancel of its caller", err)
	}
	if err := holder.Rollback(t.Context()); err != nil {
		t.Fatalf("releasing the mailbox: %v", err)
	}
	it.ingester.now = func() time.Time { return ingestNow.Add(defaultRetryWindow + time.Minute) }

	outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if mails := it.mails(); outcome != outcomeStored || mails != 1 {
		t.Errorf("ingest() past the retry window = %q with %d mails, want %q and the mail stored", outcome, mails,
			outcomeStored)
	}
	if states := it.queueStates(); len(states) != 1 || states[0] != "deleting" {
		t.Errorf("the queue holds %q, want only the raw copy of the cancelled run queued for deletion", states)
	}
}

func TestIngestStopsRecordingAFailureAtTheCleanupTimeout(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	record(t, it.pool, it.box, "file:quote", temporaryDatabaseError, ingestNow.Add(-time.Hour))
	holder := beginTx(t, it.pool)
	if _, err := holder.Exec(t.Context(), "SELECT 1 FROM plugin_email.failed_mail WHERE provider_ref = $1 FOR UPDATE",
		"file:quote"); err != nil {
		t.Fatalf("locking the failure: %v", err)
	}
	it.refuseNewMail("22023")
	it.ingester.times.cleanupTimeout = 200 * time.Millisecond
	ran := make(chan error, 1)
	go func() {
		_, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))
		ran <- err
	}()

	select {
	case err := <-ran:
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(fmt.Sprint(err), "email: set the mail aside: ") {
			t.Errorf("ingest() error = %v, want the record of the failure ended at the clean up timeout", err)
		}
	case <-time.After(rivalBudget):
		t.Errorf("ingest() still records the failure after %v, want it ended at the clean up timeout", rivalBudget)
	}
}

func TestSetAsideKeepsTheFilesOfAMailWhoseCommitLanded(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	mail, row, kept := it.prepareMail(invoiceMail("Invoice for March"), "file:invoice", false)
	if _, err := it.ingester.commitMail(t.Context(), sdk.DefaultTenantID, mail, row, kept); err != nil {
		t.Fatalf("commitMail() error = %v, want nil", err)
	}

	_, err := it.ingester.setAside(t.Context(), sdk.DefaultTenantID, mail, kept, context.Canceled)
	it.sweep()

	if !errors.Is(err, context.Canceled) {
		t.Errorf("setAside() error = %v, want the failure it is handed", err)
	}
	if states := it.queueStates(); len(states) != 0 {
		t.Errorf("the queue holds %q, want no file of the stored mail queued for deletion", states)
	}
	it.wantFilesKept(kept)
}

func TestSetAsideKeepsTheFilesOfAMailWhoseCommitLandsWhileItQueuesThem(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	mail, row, kept := it.prepareMail(invoiceMail("Invoice for March"), "file:invoice", false)
	tx := beginTx(t, it.pool)
	if _, err := it.ingester.writeMail(t.Context(), tx, sdk.DefaultTenantID, mail, row, kept); err != nil {
		t.Fatalf("writeMail() error = %v, want nil", err)
	}
	queued := make(chan error, 1)
	go func() {
		_, err := it.ingester.setAside(t.Context(), sdk.DefaultTenantID, mail, kept, context.Canceled)
		queued <- err
	}()
	it.awaitLockWaits(1)

	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("committing the mail while its files are being queued: %v", err)
	}
	if err := <-queued; !errors.Is(err, context.Canceled) {
		t.Errorf("setAside() error = %v, want the failure it is handed", err)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"deleting", "deleting"}) {
		t.Fatalf("the queue holds %q, want both files queued by a clean up that read before the commit", states)
	}
	it.sweep()

	if states := it.queueStates(); len(states) != 0 {
		t.Errorf("the queue holds %q after the sweep, want no row left", states)
	}
	it.wantFilesKept(kept)
}

func TestIngestLogsAQueueItCannotWriteAndStillSetsTheMailAside(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.refuseNewMail("22023")
	it.exec("ALTER TABLE plugin_email.pending_files ADD CONSTRAINT refuse_deleting CHECK (state <> 'deleting') NOT VALID")

	outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

	if outcome != outcomeFailed {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeFailed)
	}
	if states := it.queueStates(); !slices.Equal(states, []string{"writing"}) {
		t.Errorf("the queue holds %q, want the run's row left writing for the sweep", states)
	}
	lines := loggedLines(t, it.logged)
	if len(lines) != 1 || lines[0]["level"] != "WARN" || lines[0]["mailbox"] != it.box.id.String() ||
		lines[0]["reason"] != permanentDatabaseError {
		t.Errorf("ingest() logged %v, want one warning naming the mailbox and the reason", lines)
	}
}

func TestIngestAnswersFailedForAMailSetAsideAtItsReference(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name     string
		reason   string
		failedAt time.Time
		want     Outcome
	}{
		{name: "a permanent error a minute ago", reason: permanentDatabaseError,
			failedAt: ingestNow.Add(-time.Minute), want: outcomeFailed},
		{name: "a temporary error past the retry window", reason: temporaryDatabaseError,
			failedAt: ingestNow.Add(-defaultRetryWindow - time.Minute), want: outcomeFailed},
		{name: "a temporary error inside the retry window", reason: temporaryDatabaseError,
			failedAt: ingestNow.Add(-time.Hour), want: outcomeStored},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			record(t, it.pool, it.box, "file:quote", c.reason, c.failedAt)

			outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

			if outcome != c.want {
				t.Errorf("ingest() = %q, want %q", outcome, c.want)
			}
			failures := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.failed_mail WHERE provider_ref = $1",
				"file:quote")
			if c.want == outcomeFailed && (failures != 1 || len(it.matcher.calls()) != 0) {
				t.Errorf("the mail set aside left %d failures and was matched %d times, want its failure kept and no "+
					"read", failures, len(it.matcher.calls()))
			}
			if c.want == outcomeStored && failures != 0 {
				t.Errorf("the stored mail left %d failures, want its failure removed", failures)
			}
		})
	}
}

func TestIngestRemovesTheFailureOfAMailItLinksToAStoredCopy(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:inbox"))
	record(t, it.pool, it.box, "file:archive", temporaryDatabaseError, ingestNow.Add(-time.Hour))

	outcome := it.ingest(it.incoming(lampMail(receivedLine), "file:archive"))

	if outcome != outcomeDuplicate {
		t.Errorf("ingest() = %q, want %q", outcome, outcomeDuplicate)
	}
	if failures := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.failed_mail WHERE provider_ref = $1",
		"file:archive"); failures != 0 {
		t.Errorf("the linked copy left %d failures, want its failure removed", failures)
	}
}

func TestIngestAnswersDuplicateForAStoredMailAndDeletesTheFailureItLeftBehind(t *testing.T) {
	t.Parallel()

	for name, leave := range map[string]func(it *ingestTest){
		"the temporary failure of a commit whose answer was lost": func(it *ingestTest) {
			mail, row, kept := it.prepareMail(lampMail(), "file:quote", false)
			if _, err := it.ingester.commitMail(it.t.Context(), sdk.DefaultTenantID, mail, row, kept); err != nil {
				it.t.Fatalf("commitMail() error = %v, want nil", err)
			}
			if _, err := it.ingester.setAside(it.t.Context(), sdk.DefaultTenantID, mail, kept,
				context.Canceled); !errors.Is(err, context.Canceled) {
				it.t.Fatalf("setAside() error = %v, want %v", err, context.Canceled)
			}
		},
		"a permanent failure that would set the mail aside": func(it *ingestTest) {
			it.ingest(it.incoming(lampMail(), "file:quote"))
			record(it.t, it.pool, it.box, "file:quote", permanentDatabaseError, ingestNow)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			leave(it)
			heldFailure(t, it.pool, it.box, "file:quote")

			outcome := it.ingest(it.incoming(lampMail(), "file:quote"))

			if outcome != outcomeDuplicate {
				t.Errorf("ingest() = %q, want %q", outcome, outcomeDuplicate)
			}
			if failures := countRows(t, it.pool, "SELECT count(*) FROM plugin_email.failed_mail WHERE provider_ref = $1",
				"file:quote"); failures != 0 {
				t.Errorf("the stored mail left %d failures, want the failure of its reference deleted", failures)
			}
			if mails, matches := it.mails(), len(it.matcher.calls()); mails != 1 || matches != 1 {
				t.Errorf("the workspace holds %d mails after %d matches, want 1 of each and no read of the second run",
					mails, matches)
			}
		})
	}
}

func TestIngestReportsAFailureItCannotDeleteForAStoredMail(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.ingest(it.incoming(lampMail(), "file:quote"))
	it.exec("DROP TABLE plugin_email.failed_mail")

	outcome, err := it.ingester.ingest(t.Context(), sdk.DefaultTenantID, it.incoming(lampMail(), "file:quote"))

	if outcome != "" || pgCode(err) != "42P01" {
		t.Errorf("ingest() = %q, %v, want no outcome and the missing failed_mail table", outcome, err)
	}
}

func TestWriteMailReportsEachStatementOfANewMailThatFails(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{
		"FOR NO KEY UPDATE", "SELECT message_id FROM plugin_email.provider_refs", "AND dedupe_key = $3",
		"JOIN plugin_email.threads t ON t.id = k.thread_id", "INSERT INTO plugin_email.messages",
		"INSERT INTO plugin_email.provider_refs", "INSERT INTO plugin_email.message_contacts",
		"INSERT INTO plugin_email.attachments", "INSERT INTO plugin_email.thread_keys",
		"SELECT message_id FROM plugin_email.thread_keys", "UPDATE plugin_email.threads SET",
		"DELETE FROM plugin_email.pending_files", "DELETE FROM plugin_email.failed_mail",
		"INSERT INTO plugin_email.workspace_usage",
	} {
		t.Run(fragment, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			mail, row, kept := it.prepareMail(invoiceMail("Invoice for March"), "file:invoice", false)
			tx := batchFailingTx{failingTx{Tx: beginTx(t, it.pool), fragment: fragment}}

			_, err := it.ingester.writeMail(t.Context(), tx, sdk.DefaultTenantID, mail, row, kept)

			if pgCode(err) != undefinedFunction {
				t.Errorf("writeMail() error = %v, want the injected failure", err)
			}
		})
	}
}

func TestWriteMailReportsEachStatementOfALinkedCopyThatFails(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{
		"INSERT INTO plugin_email.provider_refs", "INSERT INTO plugin_email.message_contacts", "SET participants",
		"SET direction = 'outbound'", "'deleting', queued.size", "DELETE FROM plugin_email.failed_mail",
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

func TestWriteCopyReportsEachStatementThatFails(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{
		"FOR NO KEY UPDATE", "SELECT message_id FROM plugin_email.provider_refs", "AND dedupe_key = $3",
		"INSERT INTO plugin_email.provider_refs",
	} {
		t.Run(fragment, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			it.ingest(it.incoming(lampMail(), "file:inbox"))
			mail, err := it.ingester.readIncoming(t.Context(), it.box, it.incoming(lampMail(receivedLine), "file:archive"))
			if err != nil {
				t.Fatalf("readIncoming() error = %v, want nil", err)
			}
			tx := failingTx{Tx: beginTx(t, it.pool), fragment: fragment}

			_, err = it.ingester.writeCopy(t.Context(), tx, sdk.DefaultTenantID, mail)

			if pgCode(err) != undefinedFunction {
				t.Errorf("writeCopy() error = %v, want the injected failure", err)
			}
		})
	}
}

func TestCommitReportsATransactionItCannotBegin(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	mail, row, kept := it.prepareMail(lampMail(), "file:quote", false)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := it.ingester.commitMail(ctx, sdk.DefaultTenantID, mail, row, kept); !errors.Is(err, context.Canceled) {
		t.Errorf("commitMail() error = %v, want %v", err, context.Canceled)
	}
	if _, err := it.ingester.commitCopy(ctx, sdk.DefaultTenantID, mail); !errors.Is(err, context.Canceled) {
		t.Errorf("commitCopy() error = %v, want %v", err, context.Canceled)
	}
}

func TestPreviewAnswersTheOutcomeAMailWouldReachAndWritesNothing(t *testing.T) {
	t.Parallel()

	storeLamp := func(it *ingestTest) { it.ingest(it.incoming(lampMail(), "file:inbox")) }
	for _, c := range []struct {
		name    string
		prepare func(*ingestTest)
		raw     []byte
		ref     string
		want    Outcome
	}{
		{name: "a new mail", raw: lampMail(), ref: "file:inbox", want: outcomeStored},
		{name: "a mail too broken to read", raw: brokenMail("maria.perez@example.com"), ref: "file:broken",
			want: outcomeStoredHeadersOnly},
		{name: "a mail from a stranger", raw: strangerMail(), ref: "file:offer", want: outcomeNoKnownContact},
		{name: "a broken mail from a stranger", raw: brokenMail("stranger@example.com"), ref: "file:broken",
			want: outcomeHeadersOnlyNoKnownContact},
		{name: "a redelivery", prepare: storeLamp, raw: lampMail(), ref: "file:inbox", want: outcomeDuplicate},
		{name: "another copy", prepare: storeLamp, raw: lampMail(receivedLine), ref: "file:archive",
			want: outcomeDuplicate},
		{name: "another copy naming another contact", prepare: storeLamp,
			raw: lampMail("Cc: Ada Lovelace <ada@example.com>"), ref: "file:archive", want: outcomeLinked},
		{name: "a mail set aside", prepare: func(it *ingestTest) {
			record(it.t, it.pool, it.box, "file:inbox", permanentDatabaseError, ingestNow)
		}, raw: lampMail(), ref: "file:inbox", want: outcomeFailed},
		{name: "a stored mail whose failure stayed", prepare: func(it *ingestTest) {
			storeLamp(it)
			record(it.t, it.pool, it.box, "file:inbox", permanentDatabaseError, ingestNow)
		}, raw: lampMail(), ref: "file:inbox", want: outcomeDuplicate},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			if c.prepare != nil {
				c.prepare(it)
			}
			rows, saves, events := it.rows(sdk.DefaultTenantID), len(it.store.savedTypes()), len(it.events())

			outcome, err := it.ingester.preview(t.Context(), sdk.DefaultTenantID, it.box, it.incoming(c.raw, c.ref))

			if err != nil || outcome != c.want {
				t.Errorf("preview() = %q, %v, want %q", outcome, err, c.want)
			}
			if it.rows(sdk.DefaultTenantID) != rows || len(it.store.savedTypes()) != saves || len(it.events()) != events {
				t.Error("preview() wrote a row, saved a file or published an event, want nothing written")
			}
		})
	}
}

func TestPreviewMatchesTheOwnAddressesOfAMailboxNotYetStored(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	acme := storeWorkspace(t, it.pool)
	it.knowContacts(acme)
	box := mailbox{address: "info@example.com", aliases: []string{"sales@example.com"}}

	outcome, err := it.ingester.preview(t.Context(), acme, box, it.incoming(rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: sales@example.com", "Cc: info@example.com",
		"Subject: A second lamp", "Content-Type: text/plain", "", lampText), "file:second-lamp"))

	if err != nil || outcome != outcomeStored {
		t.Errorf("preview() = %q, %v, want %q", outcome, err, outcomeStored)
	}
	if batches := it.matcher.calls(); !reflect.DeepEqual(batches, [][]string{{"maria.perez@example.com"}}) {
		t.Errorf("the matcher was asked %q, want Maria Perez alone, the address and the alias left out", batches)
	}
	if mailboxes := mailboxRows(t, it.pool, acme); mailboxes != 0 || it.rows(acme) != 0 {
		t.Errorf("Acme holds %d mailboxes and %d rows of mail, want none", mailboxes, it.rows(acme))
	}
}

func TestPreviewAnswersAClosedWorkspaceAndAGateThatFails(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.gate.closed = true

	outcome, err := it.ingester.preview(t.Context(), sdk.DefaultTenantID, it.box, it.incoming(lampMail(), "file:quote"))

	if err != nil || outcome != outcomeWorkspaceClosed {
		t.Errorf("preview() = %q, %v, want %q", outcome, err, outcomeWorkspaceClosed)
	}

	it.gate.err = errGateDown

	if _, err := it.ingester.preview(t.Context(), sdk.DefaultTenantID, it.box,
		it.incoming(lampMail(), "file:quote")); !errors.Is(err, errGateDown) {
		t.Errorf("preview() error = %v, want %v", err, errGateDown)
	}
}

func TestPreviewReportsEachReadThatFails(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name    string
		prepare func(it *ingestTest, cancel context.CancelFunc)
		want    error
		code    string
	}{
		{name: "the provider reference", prepare: func(it *ingestTest, cancel context.CancelFunc) {
			it.gate.hook = cancel
		}, want: context.Canceled},
		{name: "the failure", prepare: func(it *ingestTest, _ context.CancelFunc) {
			it.exec("DROP TABLE plugin_email.failed_mail")
		}, code: "42P01"},
		{name: "the stored mail", prepare: func(it *ingestTest, cancel context.CancelFunc) {
			it.matcher.hook = cancel
		}, want: context.Canceled},
		{name: "the new links", prepare: func(it *ingestTest, _ context.CancelFunc) {
			it.ingest(it.incoming(lampMail(), "file:inbox"))
			it.exec("DROP TABLE plugin_email.message_contacts")
		}, code: "42P01"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.prepare(it, cancel)

			_, err := it.ingester.preview(ctx, sdk.DefaultTenantID, it.box, it.incoming(lampMail(receivedLine),
				"file:archive"))

			if c.want != nil && !errors.Is(err, c.want) || c.code != "" && pgCode(err) != c.code {
				t.Errorf("preview() error = %v, want %v%s", err, c.want, c.code)
			}
		})
	}
}

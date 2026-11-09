// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"crypto/sha256"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// testWindow is the subject window the thread store tests search in.
const testWindow = 14 * 24 * time.Hour

// hashedKey returns the thread key of kind for the id text.
func hashedKey(kind, text string) threadKey {
	return threadKey{kind: kind, hash: sha256.Sum256([]byte(text))}
}

// entryAt returns a mail entry with the subject and its key, arriving the given time after mailReceived.
func entryAt(subject string, after time.Duration) threadEntry {
	key := sha256.Sum256([]byte(strings.ToLower(subject)))
	return threadEntry{subject: subject, subjectKey: key[:], at: mailReceived.Add(after)}
}

// storeThread starts a thread of the mailbox box at entry and returns its id.
func storeThread(t *testing.T, tx pgx.Tx, box mailbox, entry threadEntry) uuid.UUID {
	t.Helper()
	id, err := insertThread(t.Context(), tx, sdk.DefaultTenantID, box.id, entry)
	if err != nil {
		t.Fatalf("insertThread() error = %v, want nil", err)
	}
	return id
}

// storeLinkedMail stores a mail of the thread linked to each of contacts and returns its id.
func storeLinkedMail(t *testing.T, tx pgx.Tx, box mailbox, thread uuid.UUID, contacts ...uuid.UUID) uuid.UUID {
	t.Helper()
	row := mailRow(box, thread)
	row.dedupeKey = nil
	id := storeMail(t, tx, sdk.DefaultTenantID, row)
	if _, err := linkContacts(t.Context(), tx, sdk.DefaultTenantID, id, contacts); err != nil {
		t.Fatalf("linkContacts() error = %v, want nil", err)
	}
	return id
}

// claimKeys points each of keys at the thread as a placeholder.
func claimKeys(t *testing.T, tx pgx.Tx, box mailbox, thread uuid.UUID, keys ...threadKey) {
	t.Helper()
	if err := claimThreadKeys(t.Context(), tx, sdk.DefaultTenantID, box.id, thread, keys); err != nil {
		t.Fatalf("claimThreadKeys() error = %v, want nil", err)
	}
}

// heldKey is where one row of plugin_email.thread_keys points.
type heldKey struct {
	thread  uuid.UUID
	message uuid.UUID
}

// keyRow reads where the key of the mailbox box points, failing the test when the mailbox holds no such key.
func keyRow(t *testing.T, tx pgx.Tx, box mailbox, key threadKey) heldKey {
	t.Helper()
	var held heldKey
	var message *uuid.UUID
	if err := tx.QueryRow(t.Context(), `SELECT thread_id, message_id FROM plugin_email.thread_keys
		WHERE tenant_id = $1 AND mailbox_id = $2 AND kind = $3 AND key = $4`,
		sdk.DefaultTenantID, box.id, key.kind, key.hash[:]).Scan(&held.thread, &message); err != nil {
		t.Fatalf("reading the key: %v", err)
	}
	if message != nil {
		held.message = *message
	}
	return held
}

// storedSpan reads the subject, the subject key and the times of one thread.
func storedSpan(t *testing.T, tx pgx.Tx, thread uuid.UUID) (string, []byte, threadSpan) {
	t.Helper()
	span := threadSpan{id: thread}
	var subject string
	var key []byte
	if err := tx.QueryRow(t.Context(), `SELECT subject, subject_key, first_message_at, last_message_at
		FROM plugin_email.threads WHERE id = $1`, thread).Scan(
		&subject, &key, &span.firstMessageAt, &span.lastMessageAt); err != nil {
		t.Fatalf("reading the thread: %v", err)
	}
	span.firstMessageAt, span.lastMessageAt = span.firstMessageAt.UTC(), span.lastMessageAt.UTC()
	return subject, key, span
}

func TestInsertThreadStartsAThreadAtItsFirstMail(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	entry := entryAt("Your quote", 0)

	id, err := insertThread(t.Context(), tx, sdk.DefaultTenantID, box.id, entry)

	if err != nil {
		t.Fatalf("insertThread() error = %v, want nil", err)
	}
	if id == uuid.Nil || id.Version() != 7 {
		t.Errorf("insertThread() id = %v, want a new UUIDv7", id)
	}
	subject, key, span := storedSpan(t, tx, id)
	want := threadSpan{id: id, firstMessageAt: entry.at, lastMessageAt: entry.at}
	if subject != entry.subject || !slices.Equal(key, entry.subjectKey) || span != want {
		t.Errorf("the new thread = %q, %x, %+v, want %q, %x, %+v", subject, key, span, entry.subject, entry.subjectKey, want)
	}
}

func TestInsertThreadStoresNoSubjectKeyAsNull(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	entry := entryAt("", 0)
	entry.subjectKey = nil

	_, key, _ := storedSpan(t, tx, storeThread(t, tx, box, entry))

	if key != nil {
		t.Errorf("subject_key = %x for a mail with no subject, want NULL", key)
	}
}

func TestInsertThreadReportsAnIDItCannotMake(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	_, err := insertThread(t.Context(), nil, sdk.DefaultTenantID, uuid.Nil, threadEntry{})

	if !errors.Is(err, errEntropy) {
		t.Errorf("insertThread() error = %v, want %v", err, errEntropy)
	}
}

func TestClaimThreadKeysStoresPlaceholders(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeThread(t, tx, box, entryAt("Your quote", 0))
	keys := []threadKey{
		hashedKey(providerThreadKind, "file:conversation"),
		hashedKey(internetMessageIDKind, "request@example.com"),
		hashedKey(threadIndexKind, "conversation guid"),
	}

	claimKeys(t, tx, box, thread, keys...)

	for _, key := range keys {
		if held := keyRow(t, tx, box, key); held != (heldKey{thread: thread}) {
			t.Errorf("the %s key points at %+v, want a placeholder of %v", key.kind, held, thread)
		}
	}
}

func TestClaimThreadKeysLeavesAKeyWithTheThreadThatClaimedItFirst(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	first := storeThread(t, tx, box, entryAt("Your quote", 0))
	second := storeThread(t, tx, box, entryAt("Re: Your quote", time.Minute))
	key := hashedKey(internetMessageIDKind, "request@example.com")
	claimKeys(t, tx, box, first, key)

	claimKeys(t, tx, box, second, key, key)

	if held := keyRow(t, tx, box, key); held.thread != first {
		t.Errorf("the key points at %v, want the thread that claimed it first, %v", held.thread, first)
	}
}

func TestHoldMessageIDKeyStoresAFreeKey(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeThread(t, tx, box, entryAt("Your quote", 0))
	mail := storeLinkedMail(t, tx, box, thread)
	key := hashedKey(internetMessageIDKind, "quote@example.com")

	if err := holdMessageIDKey(t.Context(), tx, sdk.DefaultTenantID, box.id, thread, mail, key.hash); err != nil {
		t.Fatalf("holdMessageIDKey() error = %v, want nil", err)
	}

	if held := keyRow(t, tx, box, key); held != (heldKey{thread: thread, message: mail}) {
		t.Errorf("the key points at %+v, want the mail %v of the thread %v", held, mail, thread)
	}
}

func TestHoldMessageIDKeyTakesItsKeyBackFromAMention(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	mentioning := storeThread(t, tx, box, entryAt("Delivery date", 0))
	own := storeThread(t, tx, box, entryAt("Lamp quote", time.Minute))
	mail := storeLinkedMail(t, tx, box, own)
	key := hashedKey(internetMessageIDKind, "lamp-quote@example.com")
	claimKeys(t, tx, box, mentioning, key)

	if err := holdMessageIDKey(t.Context(), tx, sdk.DefaultTenantID, box.id, own, mail, key.hash); err != nil {
		t.Fatalf("holdMessageIDKey() error = %v, want nil", err)
	}

	if held := keyRow(t, tx, box, key); held != (heldKey{thread: own, message: mail}) {
		t.Errorf("the key points at %+v, want the held mail %v of its own thread %v", held, mail, own)
	}
}

func TestHoldMessageIDKeyLeavesAKeyAnotherMailHolds(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	first := storeThread(t, tx, box, entryAt("Your quote", 0))
	second := storeThread(t, tx, box, entryAt("Your quote", time.Minute))
	holder := storeLinkedMail(t, tx, box, first)
	reuser := storeLinkedMail(t, tx, box, second)
	key := hashedKey(internetMessageIDKind, "reused@example.com")
	if err := holdMessageIDKey(t.Context(), tx, sdk.DefaultTenantID, box.id, first, holder, key.hash); err != nil {
		t.Fatalf("holdMessageIDKey() of the first mail error = %v, want nil", err)
	}

	if err := holdMessageIDKey(t.Context(), tx, sdk.DefaultTenantID, box.id, second, reuser, key.hash); err != nil {
		t.Fatalf("holdMessageIDKey() of a second mail error = %v, want nil", err)
	}

	if held := keyRow(t, tx, box, key); held != (heldKey{thread: first, message: holder}) {
		t.Errorf("the key points at %+v, want it left with the mail %v that holds it", held, holder)
	}
}

func TestReadKeyedThreadsAnswersTheThreadOfEachKeyTheMailboxHolds(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	first := storeThread(t, tx, box, entryAt("Your quote", 0))
	second := storeThread(t, tx, box, entryAt("Delivery date", time.Hour))
	mail := storeLinkedMail(t, tx, box, second)
	mentioned := hashedKey(internetMessageIDKind, "request@example.com")
	held := hashedKey(internetMessageIDKind, "delivery-date@example.com")
	claimKeys(t, tx, box, first, mentioned)
	if err := holdMessageIDKey(t.Context(), tx, sdk.DefaultTenantID, box.id, second, mail, held.hash); err != nil {
		t.Fatalf("holdMessageIDKey() error = %v, want nil", err)
	}
	unknown := hashedKey(providerThreadKind, "file:unknown")

	keyed, err := readKeyedThreads(t.Context(), tx, sdk.DefaultTenantID, box.id, []threadKey{mentioned, held, unknown})

	if err != nil {
		t.Fatalf("readKeyedThreads() error = %v, want nil", err)
	}
	want := map[threadKey]keyedThread{
		mentioned: {thread: threadSpan{id: first, firstMessageAt: mailReceived, lastMessageAt: mailReceived}},
		held: {thread: threadSpan{id: second, firstMessageAt: mailReceived.Add(time.Hour),
			lastMessageAt: mailReceived.Add(time.Hour)}, mailHeld: true},
	}
	if !reflect.DeepEqual(utcSpans(keyed), want) {
		t.Errorf("readKeyedThreads() = %+v, want %+v", keyed, want)
	}
}

// utcSpans returns keyed with every time of every span in UTC.
func utcSpans(keyed map[threadKey]keyedThread) map[threadKey]keyedThread {
	for key, thread := range keyed {
		thread.thread.firstMessageAt = thread.thread.firstMessageAt.UTC()
		thread.thread.lastMessageAt = thread.thread.lastMessageAt.UTC()
		keyed[key] = thread
	}
	return keyed
}

func TestReadKeyedThreadsLeavesOutTheKeysOfAnotherWorkspace(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	second := storeWorkspace(t, pool)
	foreign := storeMailbox(t, pool, second)
	box := storeMailbox(t, pool, sdk.DefaultTenantID)
	tx := beginTx(t, pool)
	thread, err := insertThread(t.Context(), tx, second, foreign.id, entryAt("Your quote", 0))
	if err != nil {
		t.Fatalf("insertThread() error = %v, want nil", err)
	}
	key := hashedKey(internetMessageIDKind, "request@example.com")
	if err := claimThreadKeys(t.Context(), tx, second, foreign.id, thread, []threadKey{key}); err != nil {
		t.Fatalf("claimThreadKeys() error = %v, want nil", err)
	}

	for _, mailboxID := range []uuid.UUID{box.id, foreign.id} {
		keyed, err := readKeyedThreads(t.Context(), tx, sdk.DefaultTenantID, mailboxID, []threadKey{key})

		if err != nil || len(keyed) != 0 {
			t.Errorf("readKeyedThreads() of another workspace's key = %+v, %v, want none", keyed, err)
		}
	}
}

func TestReadThreadContactsListsTheContactsOfEachThread(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	sender := storeContact(t, tx, sdk.DefaultTenantID, "Sender contact")
	copied := storeContact(t, tx, sdk.DefaultTenantID, "Copied contact")
	first := storeThread(t, tx, box, entryAt("Your quote", 0))
	second := storeThread(t, tx, box, entryAt("Delivery date", 0))
	quiet := storeThread(t, tx, box, entryAt("Site visit", 0))
	storeLinkedMail(t, tx, box, first, sender)
	storeLinkedMail(t, tx, box, first, sender, copied)
	storeLinkedMail(t, tx, box, second, copied)
	storeLinkedMail(t, tx, box, quiet)

	contacts, err := readThreadContacts(t.Context(), tx, sdk.DefaultTenantID, []uuid.UUID{first, second, quiet})

	if err != nil {
		t.Fatalf("readThreadContacts() error = %v, want nil", err)
	}
	firstContacts := []uuid.UUID{sender, copied}
	slices.SortFunc(firstContacts, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	want := map[uuid.UUID][]uuid.UUID{first: firstContacts, second: {copied}}
	if !reflect.DeepEqual(contacts, want) {
		t.Errorf("readThreadContacts() = %v, want %v", contacts, want)
	}
}

func TestReadSubjectThreadAnswersTheNewestThreadThatHoldsASender(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	sender := storeContact(t, tx, sdk.DefaultTenantID, "Sender contact")
	other := storeContact(t, tx, sdk.DefaultTenantID, "Contact of another thread")
	older := storeThread(t, tx, box, entryAt("Invoice", 0))
	newer := storeThread(t, tx, box, entryAt("Invoice", time.Hour))
	newest := storeThread(t, tx, box, entryAt("Invoice", 2*time.Hour))
	storeLinkedMail(t, tx, box, older, sender)
	storeLinkedMail(t, tx, box, newer, sender)
	storeLinkedMail(t, tx, box, newest, other)
	search := subjectSearch{mailboxID: box.id, subjectKey: entryAt("Invoice", 0).subjectKey,
		at: mailReceived.Add(3 * time.Hour), window: testWindow, senders: []uuid.UUID{sender}}

	thread, found, err := readSubjectThread(t.Context(), tx, sdk.DefaultTenantID, search)

	if err != nil || !found || thread != newer {
		t.Errorf("readSubjectThread() = %v, %t, %v, want the newest thread holding the sender, %v", thread, found, err, newer)
	}
}

func TestReadSubjectThreadLeavesOutAThreadOutsideTheWindow(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	sender := storeContact(t, tx, sdk.DefaultTenantID, "Sender contact")
	thread := storeThread(t, tx, box, entryAt("Invoice", 0))
	storeLinkedMail(t, tx, box, thread, sender)
	if err := updateThreadSpan(t.Context(), tx, sdk.DefaultTenantID, thread, entryAt("Invoice", 4*time.Hour)); err != nil {
		t.Fatalf("updateThreadSpan() error = %v, want nil", err)
	}

	for _, arrival := range []struct {
		name   string
		after  time.Duration
		inside bool
	}{
		{name: "an hour before the first mail", after: -time.Hour, inside: true},
		{name: "an hour after the last mail", after: 5 * time.Hour, inside: true},
		{name: "two hours before the first mail", after: -2 * time.Hour},
		{name: "three hours after the last mail", after: 7 * time.Hour},
		{name: "two hours from both the first and the last mail", after: 2 * time.Hour},
	} {
		search := subjectSearch{mailboxID: box.id, subjectKey: entryAt("Invoice", 0).subjectKey,
			at: mailReceived.Add(arrival.after), window: time.Hour + time.Minute, senders: []uuid.UUID{sender}}

		_, found, err := readSubjectThread(t.Context(), tx, sdk.DefaultTenantID, search)

		if err != nil || found != arrival.inside {
			t.Errorf("readSubjectThread() %s = %t, %v, want found %t", arrival.name, found, err, arrival.inside)
		}
	}
}

func TestReadSubjectThreadAnswersNoneForASubjectNoThreadHolds(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	sender := storeContact(t, tx, sdk.DefaultTenantID, "Sender contact")
	storeLinkedMail(t, tx, box, storeThread(t, tx, box, entryAt("Invoice", 0)), sender)
	search := subjectSearch{mailboxID: box.id, subjectKey: entryAt("Delivery date", 0).subjectKey,
		at: mailReceived, window: testWindow, senders: []uuid.UUID{sender}}

	thread, found, err := readSubjectThread(t.Context(), tx, sdk.DefaultTenantID, search)

	if err != nil || found || thread != uuid.Nil {
		t.Errorf("readSubjectThread() = %v, %t, %v, want none found and no error", thread, found, err)
	}
}

func TestMergeThreadsMovesMessagesAndKeysAndRemovesTheEmptyThread(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	older := storeThread(t, tx, box, entryAt("Visit day", 0))
	newer := storeThread(t, tx, box, entryAt("Visit time", time.Minute))
	kept := storeLinkedMail(t, tx, box, older)
	moved := storeLinkedMail(t, tx, box, newer)
	keptKey, movedKey := hashedKey(internetMessageIDKind, "visit-day"), hashedKey(internetMessageIDKind, "visit-time")
	claimKeys(t, tx, box, older, keptKey)
	claimKeys(t, tx, box, newer, movedKey)

	thread, err := mergeThreads(t.Context(), tx, sdk.DefaultTenantID, newer, older)

	if err != nil || thread != older {
		t.Fatalf("mergeThreads() = %v, %v, want the older thread %v kept", thread, err, older)
	}
	for _, mail := range []uuid.UUID{kept, moved} {
		if got := storedRow(t, tx, mail).threadID; got != older {
			t.Errorf("the mail %v sits in %v, want the kept thread %v", mail, got, older)
		}
	}
	for _, key := range []threadKey{keptKey, movedKey} {
		if held := keyRow(t, tx, box, key); held.thread != older {
			t.Errorf("the key %x points at %v, want the kept thread %v", key.hash, held.thread, older)
		}
	}
	if threads := countRows(t, tx, "SELECT count(*) FROM plugin_email.threads WHERE id = $1", newer); threads != 0 {
		t.Errorf("the emptied thread %v is still stored, want it removed", newer)
	}
}

func TestMergeThreadsKeepsTheEarliestSubjectAndTheLatestTime(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	older := storeThread(t, tx, box, entryAt("Visit day", 0))
	newer := storeThread(t, tx, box, entryAt("Visit time", time.Minute))
	later := entryAt("Re: Visit time", time.Hour)
	if err := updateThreadSpan(t.Context(), tx, sdk.DefaultTenantID, newer, later); err != nil {
		t.Fatalf("updateThreadSpan() error = %v, want nil", err)
	}

	thread, err := mergeThreads(t.Context(), tx, sdk.DefaultTenantID, older, newer)

	if err != nil {
		t.Fatalf("mergeThreads() error = %v, want nil", err)
	}
	subject, key, span := storedSpan(t, tx, thread)
	want := threadSpan{id: older, firstMessageAt: mailReceived, lastMessageAt: mailReceived.Add(time.Hour)}
	if subject != "Visit day" || !slices.Equal(key, entryAt("Visit day", 0).subjectKey) || span != want {
		t.Errorf("the merged thread = %q, %x, %+v, want Visit day with its key and %+v", subject, key, span, want)
	}
}

func TestMergeThreadsReportsAThreadThatDoesNotExist(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeThread(t, tx, box, entryAt("Visit day", 0))

	_, err := mergeThreads(t.Context(), tx, sdk.DefaultTenantID, thread, uuid.Must(uuid.NewV7()))

	if !errors.Is(err, pgx.ErrNoRows) || !strings.HasPrefix(err.Error(), "email: merge threads: ") {
		t.Errorf("mergeThreads() with a missing thread error = %v, want no rows named", err)
	}
}

func TestUpdateThreadSpanFollowsTheEarliestAndLatestMail(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeThread(t, tx, box, entryAt("Re: Your quote", time.Hour))
	earliest := entryAt("Your quote", 0)

	for _, entry := range []threadEntry{
		earliest, entryAt("Re: Re: Your quote", 3*time.Hour), entryAt("Re: Your quote again", 2*time.Hour),
	} {
		if err := updateThreadSpan(t.Context(), tx, sdk.DefaultTenantID, thread, entry); err != nil {
			t.Fatalf("updateThreadSpan(%q) error = %v, want nil", entry.subject, err)
		}
	}

	subject, key, span := storedSpan(t, tx, thread)
	want := threadSpan{id: thread, firstMessageAt: mailReceived, lastMessageAt: mailReceived.Add(3 * time.Hour)}
	if subject != earliest.subject || !slices.Equal(key, earliest.subjectKey) || span != want {
		t.Errorf("the thread = %q, %x, %+v, want the earliest subject %q and %+v", subject, key, span, earliest.subject, want)
	}
}

func TestThreadStatementsReportAnEndedTransaction(t *testing.T) {
	t.Parallel()

	tx := endedTx(t)
	ctx, id := t.Context(), uuid.Must(uuid.NewV7())
	key := hashedKey(internetMessageIDKind, "request@example.com")
	for prefix, call := range map[string]func() error{
		"email: insert thread: ": func() error {
			_, err := insertThread(ctx, tx, sdk.DefaultTenantID, id, threadEntry{})
			return err
		},
		"email: claim thread keys: ": func() error {
			return claimThreadKeys(ctx, tx, sdk.DefaultTenantID, id, id, []threadKey{key})
		},
		"email: hold message id key: ": func() error {
			return holdMessageIDKey(ctx, tx, sdk.DefaultTenantID, id, id, id, key.hash)
		},
		"email: read keyed threads: ": func() error {
			_, err := readKeyedThreads(ctx, tx, sdk.DefaultTenantID, id, []threadKey{key})
			return err
		},
		"email: read thread contacts: ": func() error {
			_, err := readThreadContacts(ctx, tx, sdk.DefaultTenantID, []uuid.UUID{id})
			return err
		},
		"email: read subject thread: ": func() error {
			_, _, err := readSubjectThread(ctx, tx, sdk.DefaultTenantID, subjectSearch{})
			return err
		},
		"email: merge threads: ": func() error {
			_, err := mergeThreads(ctx, tx, sdk.DefaultTenantID, id, id)
			return err
		},
		"email: update thread span: ": func() error {
			return updateThreadSpan(ctx, tx, sdk.DefaultTenantID, id, threadEntry{})
		},
	} {
		err := call()

		if !errors.Is(err, pgx.ErrTxClosed) || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("a statement on an ended transaction error = %v, want %q naming %v", err, prefix, pgx.ErrTxClosed)
		}
	}
}

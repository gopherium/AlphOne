// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

// injectedFailure is a call no database answers, sent in place of the statement a test makes fail.
const injectedFailure = "SELECT injected_failure()"

// undefinedFunction is the code PostgreSQL answers for injectedFailure.
const undefinedFunction = "42883"

// knownEmails lists the addresses the workspace of a thread test knows as contacts.
var knownEmails = []string{"maria.perez@example.com", "ada@example.com"}

// failingTx is a transaction that sends injectedFailure in place of each statement holding fragment.
type failingTx struct {
	pgx.Tx
	fragment string
}

// Exec runs sql, or injectedFailure when sql holds the fragment.
func (f failingTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, f.fragment) {
		return f.Tx.Exec(ctx, injectedFailure)
	}
	return f.Tx.Exec(ctx, sql, args...)
}

// Query reads the rows of sql, or of injectedFailure when sql holds the fragment.
func (f failingTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if strings.Contains(sql, f.fragment) {
		return f.Tx.Query(ctx, injectedFailure)
	}
	return f.Tx.Query(ctx, sql, args...)
}

// QueryRow reads one row of sql, or of injectedFailure when sql holds the fragment.
func (f failingTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, f.fragment) {
		return f.Tx.QueryRow(ctx, injectedFailure)
	}
	return f.Tx.QueryRow(ctx, sql, args...)
}

// threadTest is one thread test: its transaction, its mailbox and the contacts its workspace knows by email.
type threadTest struct {
	t        *testing.T
	tx       pgx.Tx
	box      mailbox
	contacts map[string]uuid.UUID
}

// newThreadTest returns a thread test over a migrated database whose default workspace knows every known email.
func newThreadTest(t *testing.T) *threadTest {
	t.Helper()
	tx, box := newMailbox(t)
	contacts := map[string]uuid.UUID{}
	for _, email := range knownEmails {
		contacts[email] = storeContact(t, tx, sdk.DefaultTenantID, email)
	}
	return &threadTest{t: t, tx: tx, box: box, contacts: contacts}
}

// arrival says how one mail of a thread test reaches the mailbox.
type arrival struct {
	providerThread string
	sentCopy       bool
	automated      bool
	after          time.Duration
}

// threadedMail returns a plain text mail holding the header lines given, dated quoteDate.
func threadedMail(headers ...string) []byte {
	return rawMail(slices.Concat(headers, []string{"Date: " + quoteDate, "Content-Type: text/plain", "", "Hello"})...)
}

// threadIndex returns a Thread-Index value whose conversation GUID repeats the byte guid.
func threadIndex(guid byte) string {
	return base64.StdEncoding.EncodeToString(append([]byte{1, 1, 0xd9, 0x3c, 0, 0}, bytes.Repeat([]byte{guid}, 16)...))
}

// mail returns raw as the thread steps read it, matched to the known contacts it names as ingest matches them.
func (tt *threadTest) mail(raw []byte, how arrival) threadMail {
	tt.t.Helper()
	m := parsed(tt.t, raw)
	addresses := slices.Concat([]message.Address{m.From}, m.ReplyTo, m.To, m.Cc)
	if how.sentCopy {
		addresses = append(addresses, m.Bcc...)
	}
	matched := map[string]uuid.UUID{}
	for _, address := range addresses {
		if contact, known := tt.contacts[address.Email]; known {
			matched[address.Email] = contact
		}
	}
	return threadMail{
		mailboxID: tt.box.id, message: m, subject: m.Subject, receivedAt: mailReceived.Add(how.after),
		sentCopy: how.sentCopy, automated: how.automated, contacts: matched,
		keys: keysOf(m, how.providerThread, defaultThreadReferencesMax),
	}
}

// store chooses the thread of mail and stores mail in it the way ingest does, returning the thread and the mail.
func (tt *threadTest) store(mail threadMail) (uuid.UUID, uuid.UUID) {
	tt.t.Helper()
	ctx := tt.t.Context()
	thread, err := chooseThread(ctx, tt.tx, sdk.DefaultTenantID, mail, testWindow)
	if err != nil {
		tt.t.Fatalf("chooseThread() error = %v, want nil", err)
	}
	row := mailRow(tt.box, thread)
	row.dedupeKey, row.subject, row.receivedAt = nil, mail.subject, mail.receivedAt
	id := storeMail(tt.t, tt.tx, sdk.DefaultTenantID, row)
	if _, err := linkContacts(ctx, tt.tx, sdk.DefaultTenantID, id, mail.namedContacts()); err != nil {
		tt.t.Fatalf("linkContacts() error = %v, want nil", err)
	}
	if err := claimMailKeys(ctx, tt.tx, sdk.DefaultTenantID, mail, thread, id); err != nil {
		tt.t.Fatalf("claimMailKeys() error = %v, want nil", err)
	}
	entry := threadEntry{subject: mail.subject, subjectKey: subjectKeyOf(mail.message.Subject), at: mail.receivedAt}
	if err := updateThreadSpan(ctx, tt.tx, sdk.DefaultTenantID, thread, entry); err != nil {
		tt.t.Fatalf("updateThreadSpan() error = %v, want nil", err)
	}
	return thread, id
}

// reach stores the mail raw arriving as how and returns its thread.
func (tt *threadTest) reach(raw []byte, how arrival) uuid.UUID {
	tt.t.Helper()
	thread, _ := tt.store(tt.mail(raw, how))
	return thread
}

// threads counts the threads of the mailbox.
func (tt *threadTest) threads() int {
	tt.t.Helper()
	return countRows(tt.t, tt.tx, "SELECT count(*) FROM plugin_email.threads WHERE mailbox_id = $1", tt.box.id)
}

// failOn chooses the thread of mail while each statement holding fragment fails.
func (tt *threadTest) failOn(fragment string, mail threadMail) error {
	tt.t.Helper()
	_, err := chooseThread(tt.t.Context(), failingTx{Tx: tt.tx, fragment: fragment}, sdk.DefaultTenantID, mail, testWindow)
	return err
}

// stepsInOrder stores four mails of the known sender, one for each step, and returns their threads by step.
func (tt *threadTest) stepsInOrder() map[string]uuid.UUID {
	tt.t.Helper()
	return map[string]uuid.UUID{
		"provider thread": tt.reach(threadedMail("From: maria.perez@example.com", "To: info@example.com",
			"Subject: Lamp", "Message-ID: <lamp@example.com>"), arrival{providerThread: "file:conversation"}),
		"reply id": tt.reach(threadedMail("From: maria.perez@example.com", "To: info@example.com",
			"Subject: Desk", "Message-ID: <desk@example.com>"), arrival{}),
		"Thread-Index": tt.reach(threadedMail("From: maria.perez@example.com", "To: info@example.com",
			"Subject: Chair", "Message-ID: <chair@example.com>", "Thread-Index: "+threadIndex(0x11)), arrival{}),
		"subject": tt.reach(threadedMail("From: maria.perez@example.com", "To: info@example.com",
			"Subject: Invoice", "Message-ID: <invoice@example.com>"), arrival{}),
	}
}

func TestChooseThreadTakesTheStepsInOrder(t *testing.T) {
	t.Parallel()

	for _, step := range []struct {
		name    string
		headers []string
		how     arrival
	}{
		{name: "provider thread", how: arrival{providerThread: "file:conversation", after: time.Minute},
			headers: []string{"In-Reply-To: <desk@example.com>", "Thread-Index: " + threadIndex(0x11), "Subject: Re: Invoice"}},
		{name: "reply id", how: arrival{after: time.Minute},
			headers: []string{"In-Reply-To: <desk@example.com>", "Thread-Index: " + threadIndex(0x11), "Subject: Re: Invoice"}},
		{name: "Thread-Index", how: arrival{after: time.Minute},
			headers: []string{"Thread-Index: " + threadIndex(0x11), "Subject: Re: Invoice"}},
		{name: "subject", how: arrival{after: time.Minute}, headers: []string{"Subject: Re: Invoice"}},
		{name: "new thread", how: arrival{after: time.Minute}, headers: []string{"Subject: Re: Shelf"}},
	} {
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()

			tt := newThreadTest(t)
			threads := tt.stepsInOrder()

			got := tt.reach(threadedMail(slices.Concat([]string{"From: maria.perez@example.com", "To: info@example.com",
				"Message-ID: <next@example.com>"}, step.headers)...), step.how)

			want, found := threads[step.name]
			if !found && slices.Contains(slices.Collect(maps.Values(threads)), got) {
				t.Errorf("a mail no step finds a thread for joined %v, want a new thread", got)
			}
			if found && got != want {
				t.Errorf("the mail joined %v, want the thread the %s step finds, %v", got, step.name, want)
			}
		})
	}
}

func TestChooseThreadJoinsTheNearestReplyThreadFirst(t *testing.T) {
	t.Parallel()

	for _, reply := range []struct {
		name    string
		headers []string
		want    string
	}{
		{name: "In-Reply-To before References", want: "second",
			headers: []string{"In-Reply-To: <second@example.com>", "References: <second@example.com> <first@example.com>"}},
		{name: "the last References id first", want: "second",
			headers: []string{"References: <first@example.com> <second@example.com>"}},
		{name: "an earlier References id when the last leads nowhere", want: "first",
			headers: []string{"References: <first@example.com> <unknown@example.com>"}},
	} {
		t.Run(reply.name, func(t *testing.T) {
			t.Parallel()

			tt := newThreadTest(t)
			threads := map[string]uuid.UUID{
				"first": tt.reach(threadedMail("From: maria.perez@example.com", "To: info@example.com",
					"Subject: Site visit", "Message-ID: <first@example.com>"), arrival{}),
				"second": tt.reach(threadedMail("From: ada@example.com", "To: info@example.com",
					"Subject: Delivery date", "Message-ID: <second@example.com>"), arrival{}),
			}

			got := tt.reach(threadedMail(slices.Concat([]string{"From: colleague@example.com", "To: info@example.com",
				"Cc: maria.perez@example.com, ada@example.com", "Subject: Both", "Message-ID: <both@example.com>"},
				reply.headers)...), arrival{after: time.Minute})

			if got != threads[reply.want] {
				t.Errorf("the reply joined %v, want the %s thread %v", got, reply.want, threads[reply.want])
			}
			if count := tt.threads(); count != 2 {
				t.Errorf("the mailbox holds %d threads, want 2 kept apart", count)
			}
		})
	}
}

func TestCapIDsKeepsTheFirstAndTheNewest(t *testing.T) {
	t.Parallel()

	ids := []string{"r1", "r2", "r3", "r4", "r5", "r6"}
	for limit, want := range map[int][]string{
		1: {"r1"},
		3: {"r1", "r5", "r6"},
		6: ids,
		9: ids,
	} {
		if got := capIDs(ids, limit); !slices.Equal(got, want) {
			t.Errorf("capIDs(%q, %d) = %q, want %q", ids, limit, got, want)
		}
	}
}

// idKeys returns the internet_message_id key of each id.
func idKeys(ids ...string) []threadKey {
	keys := make([]threadKey, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, hashedKey(internetMessageIDKind, id))
	}
	return keys
}

func TestKeysOfCutsTheReferencesToTheFirstAndTheNewest(t *testing.T) {
	t.Parallel()

	m := parsed(t, threadedMail("From: maria.perez@example.com", "Message-ID: <own@example.com>",
		"In-Reply-To: <r6@example.com>",
		"References: <r1@example.com> <r2@example.com> <r3@example.com> <r4@example.com> <r5@example.com> <r6@example.com>"))

	keys := keysOf(m, "", 3)

	if want := idKeys("r6@example.com", "r5@example.com", "r1@example.com"); !reflect.DeepEqual(keys.replies, want) {
		t.Errorf("keysOf() replies = %x, want r6, r5 and r1, %x", keys.replies, want)
	}
}

func TestKeysOfSkipsABareAddressAndTheMailsOwnID(t *testing.T) {
	t.Parallel()

	m := parsed(t, threadedMail("From: maria.perez@example.com", "Cc: ada@example.com",
		"Message-ID: <own@example.com>", "In-Reply-To: <parent@example.com>",
		"References: <Maria.Perez@example.com> <own@example.com> <ada@example.com> <parent@example.com>"))

	keys := keysOf(m, "", defaultThreadReferencesMax)

	if want := idKeys("parent@example.com"); !reflect.DeepEqual(keys.replies, want) {
		t.Errorf("keysOf() replies = %x, want only parent@example.com, %x", keys.replies, want)
	}
}

func TestKeysOfHashesEachIDOfTheMail(t *testing.T) {
	t.Parallel()

	m := parsed(t, threadedMail("From: maria.perez@example.com", "Message-ID: <own@example.com>",
		"Thread-Index: "+threadIndex(0x22)))

	keys := keysOf(m, "file:conversation", defaultThreadReferencesMax)

	guid := bytes.Repeat([]byte{0x22}, 16)
	want := mailKeys{
		provider: &threadKey{kind: providerThreadKind, hash: sha256.Sum256([]byte("file:conversation"))},
		own:      &threadKey{kind: internetMessageIDKind, hash: sha256.Sum256([]byte("own@example.com"))},
		index:    &threadKey{kind: threadIndexKind, hash: sha256.Sum256(guid)},
	}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("keysOf() = %+v, want %+v", keys, want)
	}
}

func TestKeysOfLeavesOutAnIDTheMailLacks(t *testing.T) {
	t.Parallel()

	keys := keysOf(parsed(t, threadedMail("From: maria.perez@example.com")), "", defaultThreadReferencesMax)

	if !reflect.DeepEqual(keys, mailKeys{}) {
		t.Errorf("keysOf() of a mail with no id = %+v, want no key", keys)
	}
}

func TestSubjectKeyOfMatchesASubjectInAnyCaseAndTakesNoneForAnEmptySubject(t *testing.T) {
	t.Parallel()

	upper, lower := subjectKeyOf("INVOICE"), subjectKeyOf("Re: invoice")

	if upper == nil || !bytes.Equal(upper, lower) {
		t.Errorf("subjectKeyOf(INVOICE) = %x, subjectKeyOf(Re: invoice) = %x, want one key", upper, lower)
	}
	if key := subjectKeyOf("Re: "); key != nil {
		t.Errorf("subjectKeyOf(Re: ) = %x, want none", key)
	}
}

func TestChooseThreadMatchesASubjectInAnyCase(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	invoice := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: INVOICE"), arrival{})

	got := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Re: invoice"), arrival{after: time.Hour})

	if got != invoice {
		t.Errorf("Re: invoice joined %v, want the INVOICE thread %v", got, invoice)
	}
}

func TestChooseThreadStartsANewThreadForAnEmptySubject(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Re:"), arrival{})

	tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Re:"), arrival{after: time.Hour})

	if count := tt.threads(); count != 2 {
		t.Errorf("two mails with an empty subject sit in %d threads, want 2", count)
	}
}

func TestChooseThreadFallsThroughAHitThatFailsTheContactRule(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	tt.reach(threadedMail("From: ada@example.com", "Subject: Delivery date", "Message-ID: <delivery@example.com>"),
		arrival{})
	invoice := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice"), arrival{})

	got := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Re: Invoice",
		"In-Reply-To: <delivery@example.com>"), arrival{after: time.Hour})

	if got != invoice {
		t.Errorf("the mail joined %v, want the reply hit to fall through to the subject thread %v", got, invoice)
	}
}

func TestChooseThreadTakesTheOwnIDOnlyAsAPlaceholder(t *testing.T) {
	t.Parallel()

	t.Run("a placeholder", func(t *testing.T) {
		t.Parallel()

		tt := newThreadTest(t)
		mentioning := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Lamp",
			"References: <later@example.com>"), arrival{})

		got := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Desk",
			"Message-ID: <later@example.com>"), arrival{after: time.Hour})

		if got != mentioning {
			t.Errorf("the mentioned mail joined %v, want the thread that mentions it, %v", got, mentioning)
		}
	})
	t.Run("an id another mail holds", func(t *testing.T) {
		t.Parallel()

		tt := newThreadTest(t)
		holder := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Lamp",
			"Message-ID: <reused@example.com>"), arrival{})

		got := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Desk",
			"Message-ID: <reused@example.com>"), arrival{after: time.Hour})

		if got == holder {
			t.Errorf("a second mail with a held id joined the holder's thread %v, want a new thread", got)
		}
		if held := keyRow(t, tt.tx, tt.box, hashedKey(internetMessageIDKind, "reused@example.com")); held.thread != holder {
			t.Errorf("the held id points at %v, want it left with the first mail's thread %v", held.thread, holder)
		}
	})
}

func TestChooseThreadNeverTakesTheSubjectStepForAnAutomatedMail(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	invoice := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice"), arrival{})

	got := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Re: Invoice"),
		arrival{automated: true, after: time.Hour})

	if got == invoice {
		t.Errorf("an automated Re: Invoice joined %v, want a new thread", got)
	}
}

func TestStepsOneThreeAndFourUseTheSenderContacts(t *testing.T) {
	t.Parallel()

	for _, step := range []struct {
		name    string
		headers []string
		how     arrival
	}{
		{name: "provider thread", how: arrival{providerThread: "file:conversation", after: time.Hour}},
		{name: "Thread-Index", headers: []string{"Thread-Index: " + threadIndex(0x33)}, how: arrival{after: time.Hour}},
		{name: "subject", headers: []string{"Subject: Re: Invoice"}, how: arrival{after: time.Hour}},
	} {
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()

			for sender, joins := range map[string]bool{"maria.perez@example.com": true, "ada@example.com": false} {
				tt := newThreadTest(t)
				thread := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice",
					"Thread-Index: "+threadIndex(0x33)), arrival{providerThread: "file:conversation"})

				got := tt.reach(threadedMail(slices.Concat([]string{"From: " + sender, "Cc: maria.perez@example.com"},
					step.headers)...), step.how)

				if (got == thread) != joins {
					t.Errorf("a mail from %s copying the thread's contact joined it = %t, want %t", sender, got == thread, joins)
				}
			}
		})
	}
}

func TestStepTwoUsesTheNamedContacts(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	site := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Site visit",
		"Message-ID: <site@example.com>"), arrival{})

	got := tt.reach(threadedMail("From: colleague@example.com", "Cc: maria.perez@example.com",
		"Subject: Re: Site visit", "In-Reply-To: <site@example.com>"), arrival{after: time.Hour})

	if got != site {
		t.Errorf("a reply copying the thread's contact joined %v, want the thread %v", got, site)
	}
}

func TestASentCopysSenderContactsAreItsRecipients(t *testing.T) {
	t.Parallel()

	for _, copied := range []struct {
		name    string
		headers []string
		how     arrival
		joins   bool
	}{
		{name: "a Sent copy to the contact", headers: []string{"From: info@example.com", "To: maria.perez@example.com"},
			how: arrival{sentCopy: true}, joins: true},
		{name: "a Sent copy blind copied to the contact",
			headers: []string{"From: info@example.com", "To: colleague@example.com", "Bcc: maria.perez@example.com"},
			how:     arrival{sentCopy: true}, joins: true},
		{name: "an inbound mail from the mailbox's own address",
			headers: []string{"From: info@example.com", "To: maria.perez@example.com"}},
	} {
		t.Run(copied.name, func(t *testing.T) {
			t.Parallel()

			tt := newThreadTest(t)
			thread := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Lamp"),
				arrival{providerThread: "file:conversation"})
			copied.how.providerThread, copied.how.after = "file:conversation", time.Hour

			got := tt.reach(threadedMail(append(copied.headers, "Subject: Lamp")...), copied.how)

			if (got == thread) != copied.joins {
				t.Errorf("%s joined the provider thread = %t, want %t", copied.name, got == thread, copied.joins)
			}
		})
	}
}

func TestAMailNamingTwoThreadsThatShareAContactMergesThem(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	site := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Site visit",
		"Message-ID: <site@example.com>"), arrival{})
	delivery := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Delivery date",
		"Message-ID: <delivery@example.com>"), arrival{after: time.Minute})
	other := tt.reach(threadedMail("From: ada@example.com", "Subject: Shelf", "Message-ID: <shelf@example.com>"),
		arrival{after: time.Minute})

	got := tt.reach(threadedMail("From: maria.perez@example.com", "Cc: ada@example.com", "Subject: Both plans",
		"In-Reply-To: <delivery@example.com>", "References: <shelf@example.com> <site@example.com> <delivery@example.com>"),
		arrival{after: time.Hour})

	if got != site {
		t.Errorf("the mail joined %v, want the older thread %v that the merge keeps", got, site)
	}
	if threads := countRows(t, tt.tx, "SELECT count(*) FROM plugin_email.threads WHERE id = ANY($1)",
		[]uuid.UUID{site, delivery, other}); threads != 2 {
		t.Errorf("%d of the three threads remain, want the two of one contact merged and the other kept", threads)
	}
}

func TestTwoThreadsWithNoSharedContactDoNotMerge(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	site := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Site visit",
		"Message-ID: <site@example.com>"), arrival{})
	tt.reach(threadedMail("From: ada@example.com", "Subject: Delivery date", "Message-ID: <delivery@example.com>"),
		arrival{after: time.Minute})

	got := tt.reach(threadedMail("From: colleague@example.com", "Cc: maria.perez@example.com, ada@example.com",
		"Subject: Re: Site visit", "In-Reply-To: <site@example.com>",
		"References: <delivery@example.com> <site@example.com>"), arrival{after: time.Hour})

	if got != site {
		t.Errorf("the mail joined %v, want the nearest thread %v", got, site)
	}
	if threads := tt.threads(); threads != 2 {
		t.Errorf("the mailbox holds %d threads, want the two threads of two contacts kept apart", threads)
	}
}

func TestAProviderHitOutsideTheWindowCountsOnlyWithReplyIDsOrAThreadIndex(t *testing.T) {
	t.Parallel()

	for _, late := range []struct {
		name    string
		headers []string
		joins   bool
	}{
		{name: "no reply id"},
		{name: "a reply id", headers: []string{"In-Reply-To: <unknown@example.com>"}, joins: true},
		{name: "a Thread-Index", headers: []string{"Thread-Index: " + threadIndex(0x44)}, joins: true},
	} {
		t.Run(late.name, func(t *testing.T) {
			t.Parallel()

			tt := newThreadTest(t)
			thread := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice"),
				arrival{providerThread: "file:conversation"})

			got := tt.reach(threadedMail(append([]string{"From: maria.perez@example.com", "Subject: Invoice"},
				late.headers...)...), arrival{providerThread: "file:conversation", after: 15 * 24 * time.Hour})

			if (got == thread) != late.joins {
				t.Errorf("a late mail with %s joined the provider thread = %t, want %t", late.name, got == thread, late.joins)
			}
		})
	}
}

func TestClaimMailKeysWritesTheKeysOfAMailWithNoID(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)

	thread := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Lamp",
		"References: <parent@example.com>", "Thread-Index: "+threadIndex(0x55)), arrival{providerThread: "file:lamp"})

	for _, key := range []threadKey{
		hashedKey(providerThreadKind, "file:lamp"),
		hashedKey(internetMessageIDKind, "parent@example.com"),
		{kind: threadIndexKind, hash: sha256.Sum256(bytes.Repeat([]byte{0x55}, 16))},
	} {
		if held := keyRow(t, tt.tx, tt.box, key); held != (heldKey{thread: thread}) {
			t.Errorf("the %s key points at %+v, want a placeholder of %v", key.kind, held, thread)
		}
	}
}

func TestChooseThreadReportsAStatementThatFails(t *testing.T) {
	t.Parallel()

	for _, failing := range []struct {
		fragment string
		headers  []string
	}{
		{fragment: "FROM plugin_email.thread_keys k", headers: []string{"Subject: Shelf"}},
		{fragment: "SELECT DISTINCT m.thread_id", headers: []string{"Subject: Shelf"}},
		{fragment: "WITH pair AS", headers: []string{"Subject: Shelf",
			"References: <desk@example.com> <lamp@example.com>"}},
		{fragment: "t.subject_key = $3", headers: []string{"Subject: Re: Invoice"}},
		{fragment: "INSERT INTO plugin_email.threads", headers: []string{"Subject: Shelf"}},
	} {
		t.Run(failing.fragment, func(t *testing.T) {
			t.Parallel()

			tt := newThreadTest(t)
			tt.stepsInOrder()
			mail := tt.mail(threadedMail(append([]string{"From: maria.perez@example.com"}, failing.headers...)...),
				arrival{after: time.Hour})

			if err := tt.failOn(failing.fragment, mail); pgCode(err) != undefinedFunction {
				t.Errorf("chooseThread() while %q fails error = %v, want the failure answered", failing.fragment, err)
			}
		})
	}
}

func TestClaimMailKeysReportsAStatementThatFails(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{"DO UPDATE", "DO NOTHING"} {
		t.Run(fragment, func(t *testing.T) {
			t.Parallel()

			tt := newThreadTest(t)
			mail := tt.mail(threadedMail("From: maria.perez@example.com", "Subject: Lamp",
				"Message-ID: <lamp@example.com>", "References: <parent@example.com>"), arrival{})
			thread, stored := tt.store(mail)

			err := claimMailKeys(t.Context(), failingTx{Tx: tt.tx, fragment: fragment}, sdk.DefaultTenantID, mail,
				thread, stored)

			if pgCode(err) != undefinedFunction {
				t.Errorf("claimMailKeys() while %q fails error = %v, want the failure answered", fragment, err)
			}
		})
	}
}

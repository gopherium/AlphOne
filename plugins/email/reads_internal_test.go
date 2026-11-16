// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

// readPage is a page wide enough to hold every thread or mail a read test stores.
const readPage = 10

// firstSender is how a light read names the people of a mail, to pick its first sender alone.
const firstSender = "jsonb_path_query_first(m.participants"

// recordingTx is a transaction that keeps the text of every statement it reads.
type recordingTx struct {
	pgx.Tx
	sent *[]string
}

// Query keeps sql and reads its rows.
func (r recordingTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	*r.sent = append(*r.sent, sql)
	return r.Tx.Query(ctx, sql, args...)
}

// threadIDs returns the id of each of threads, in order.
func threadIDs(threads []threadView) []uuid.UUID {
	ids := make([]uuid.UUID, len(threads))
	for i, thread := range threads {
		ids[i] = thread.id
	}
	return ids
}

// mailIDs returns the id of each of mails, in order.
func mailIDs(mails []mailView) []uuid.UUID {
	ids := make([]uuid.UUID, len(mails))
	for i, mail := range mails {
		ids[i] = mail.id
	}
	return ids
}

// byIDDescending returns ids from the largest to the smallest.
func byIDDescending(ids ...uuid.UUID) []uuid.UUID {
	return slices.SortedFunc(slices.Values(ids), func(a, b uuid.UUID) int { return bytes.Compare(b[:], a[:]) })
}

// soleMail returns the only mail of mails, the zero mail when it holds none or several.
func soleMail(mails []mailView) mailView {
	if len(mails) != 1 {
		return mailView{}
	}
	return mails[0]
}

// inUTC returns mail with its times in UTC.
func inUTC(mail mailView) mailView {
	mail.receivedAt = mail.receivedAt.UTC()
	if mail.sentAt != nil {
		sent := mail.sentAt.UTC()
		mail.sentAt = &sent
	}
	return mail
}

func TestReadInboxListsTheThreadsByTheirNewestMailWithTiesByID(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	quote := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	mailIn(t, tt.tx, tt.box, quote, 0)
	mailIn(t, tt.tx, tt.box, quote, 5*time.Hour)
	invoice := storeThread(t, tt.tx, tt.box, entryAt("Invoice", 3*time.Hour))
	mailIn(t, tt.tx, tt.box, invoice, 3*time.Hour)
	var tied []uuid.UUID
	for _, subject := range []string{"Site visit", "Lamp order"} {
		thread := storeThread(t, tt.tx, tt.box, entryAt(subject, time.Hour))
		mailIn(t, tt.tx, tt.box, thread, time.Hour)
		tied = append(tied, thread)
	}
	want := slices.Concat([]uuid.UUID{quote, invoice}, byIDDescending(tied...))

	whole, err := readInbox(t.Context(), tt.tx, sdk.DefaultTenantID, readPage, 0)
	page, pageErr := readInbox(t.Context(), tt.tx, sdk.DefaultTenantID, 2, 1)

	if err != nil || !slices.Equal(threadIDs(whole.items), want) || whole.total != 4 {
		t.Errorf("readInbox() = %v of %d, %v, want %v of 4", threadIDs(whole.items), whole.total, err, want)
	}
	if pageErr != nil || !slices.Equal(threadIDs(page.items), want[1:3]) || page.total != 4 {
		t.Errorf("readInbox() of 2 after 1 = %v of %d, %v, want %v of 4",
			threadIDs(page.items), page.total, pageErr, want[1:3])
	}
}

func TestReadInboxAnswersTheNewestMailTheCountAndTheFilesOfEachThread(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	quote := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	storeFiles(t, tt.tx, mailIn(t, tt.tx, tt.box, quote, 0), fileAt(0, "quote.pdf", false))
	newest := mailIn(t, tt.tx, tt.box, quote, 2*time.Hour)
	mailIn(t, tt.tx, tt.box, quote, time.Hour)
	lamp := storeThread(t, tt.tx, tt.box, entryAt("Lamp", 4*time.Hour))
	lampOnly := mailIn(t, tt.tx, tt.box, lamp, 4*time.Hour)
	storeFiles(t, tt.tx, lampOnly, fileAt(0, "lamp.png", true))
	visit := storeThread(t, tt.tx, tt.box, entryAt("Site visit", 3*time.Hour))
	visitOnly := mailIn(t, tt.tx, tt.box, visit, 3*time.Hour)

	page, err := readInbox(t.Context(), tt.tx, sdk.DefaultTenantID, readPage, 0)

	if err != nil {
		t.Fatalf("readInbox() error = %v, want nil", err)
	}
	type summary struct {
		id, latest uuid.UUID
		count      int
		files      bool
	}
	var got []summary
	for _, thread := range page.items {
		got = append(got, summary{thread.id, thread.latest.id, thread.messageCount, thread.hasAttachments})
	}
	want := []summary{{lamp, lampOnly, 1, false}, {visit, visitOnly, 1, false}, {quote, newest, 3, true}}
	if !slices.Equal(got, want) {
		t.Errorf("readInbox() threads = %+v, want %+v", got, want)
	}
}

func TestEveryLightReadAnswersEachColumnOfTheMail(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	id := mailIn(t, tt.tx, tt.box, thread, 0, func(row *messageRow) {
		row.direction, row.bodyState, row.automation = outbound, bodyCut, message.AutoReply
		row.encrypted, row.winmail, row.rawState = true, true, fileTooLarge
	})
	maria := tt.contacts["maria.perez@example.com"]
	linkMail(t, tt.tx, id, maria)
	sent := mailReceived.Add(-time.Minute)
	want := mailView{
		id: id, threadID: thread, direction: outbound, subject: "Your quote",
		from:   &participant{Role: "from", Name: "Maria Perez", Email: "maria.perez@example.com"},
		sentAt: &sent, receivedAt: mailReceived, snippet: "Could you send me a quote?", bodyState: bodyCut,
		skippedParts: 1, automation: "auto-reply", encrypted: true, signed: true, winmail: true,
		inviteMethod: "REQUEST", rawState: fileTooLarge, rawSize: 1200,
	}
	ctx, tenant := t.Context(), sdk.DefaultTenantID

	for name, read := range map[string]func() (mailView, error){
		"readInbox": func() (mailView, error) {
			page, err := readInbox(ctx, tt.tx, tenant, readPage, 0)
			if len(page.items) != 1 {
				return mailView{}, err
			}
			return page.items[0].latest, err
		},
		"readThread": func() (mailView, error) {
			held, _, err := readThread(ctx, tt.tx, tenant, thread)
			return held.latest, err
		},
		"readThreadMails": func() (mailView, error) {
			mails, err := readThreadMails(ctx, tt.tx, tenant, thread, readPage, 0)
			return soleMail(mails), err
		},
		"readThreadMail": func() (mailView, error) {
			mail, _, err := readThreadMail(ctx, tt.tx, tenant, thread, id)
			return mail, err
		},
		"readContactMails": func() (mailView, error) {
			page, err := readContactMails(ctx, tt.tx, tenant, maria, readPage, 0)
			return soleMail(page.items), err
		},
	} {
		got, err := read()

		if err != nil || !reflect.DeepEqual(inUTC(got), want) {
			t.Errorf("%s() mail = %+v, %v, want %+v", name, got, err, want)
		}
	}
}

func TestTheLightReadsAnswerTheFirstSenderAndNoneForAMailThatNamesNone(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	twoSenders := mailIn(t, tt.tx, tt.box, thread, time.Hour, func(row *messageRow) {
		row.participants = []participant{
			{Role: "to", Email: "info@example.com"},
			{Role: "from", Name: "Maria Perez", Email: "maria.perez@example.com"},
			{Role: "from", Name: "M. Perez", Email: "maria.perez@example.com"},
		}
	})
	noSender := mailIn(t, tt.tx, tt.box, thread, 0, func(row *messageRow) {
		row.participants = []participant{{Role: "to", Email: "info@example.com"}}
	})

	mails, err := readThreadMails(t.Context(), tt.tx, sdk.DefaultTenantID, thread, readPage, 0)

	if err != nil || !slices.Equal(mailIDs(mails), []uuid.UUID{twoSenders, noSender}) {
		t.Fatalf("readThreadMails() = %v, %v, want %v", mailIDs(mails), err, []uuid.UUID{twoSenders, noSender})
	}
	first := participant{Role: "from", Name: "Maria Perez", Email: "maria.perez@example.com"}
	if mails[0].from == nil || *mails[0].from != first {
		t.Errorf("the sender of a mail naming two = %+v, want the first %+v", mails[0].from, first)
	}
	if mails[1].from != nil {
		t.Errorf("the sender of a mail naming none = %+v, want nil", mails[1].from)
	}
}

func TestReadThreadAnswersOneThreadAndNoneForAnUnknownOrMergedID(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	kept := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	mailIn(t, tt.tx, tt.box, kept, 0)
	merged := storeThread(t, tt.tx, tt.box, entryAt("Your quote", time.Hour))
	later := mailIn(t, tt.tx, tt.box, merged, time.Hour)
	if _, err := mergeThreads(t.Context(), tt.tx, sdk.DefaultTenantID, kept, merged); err != nil {
		t.Fatalf("mergeThreads() error = %v, want nil", err)
	}

	thread, found, err := readThread(t.Context(), tt.tx, sdk.DefaultTenantID, kept)

	if err != nil || !found || thread.id != kept || thread.subject != "Your quote" ||
		!thread.firstMessageAt.Equal(mailReceived) || !thread.lastMessageAt.Equal(mailReceived.Add(time.Hour)) ||
		thread.messageCount != 2 || thread.latest.id != later {
		t.Errorf("readThread() = %+v, %t, %v, want the kept thread of 2 mails ending at %v", thread, found, err, later)
	}
	for name, id := range map[string]uuid.UUID{"the merged thread": merged, "an unknown id": uuid.Must(uuid.NewV7())} {
		held, found, err := readThread(t.Context(), tt.tx, sdk.DefaultTenantID, id)

		if err != nil || found {
			t.Errorf("readThread() of %s = %+v, %t, %v, want none found", name, held, found, err)
		}
	}
}

func TestReadThreadMailsListsTheMailNewestFirstWithTiesByIDAfterTheOffset(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	oldest := mailIn(t, tt.tx, tt.box, thread, 0)
	newer := mailIn(t, tt.tx, tt.box, thread, 2*time.Hour)
	older := mailIn(t, tt.tx, tt.box, thread, time.Hour)
	tied := byIDDescending(mailIn(t, tt.tx, tt.box, thread, 3*time.Hour), mailIn(t, tt.tx, tt.box, thread, 3*time.Hour))
	other := storeThread(t, tt.tx, tt.box, entryAt("Invoice", 0))
	mailIn(t, tt.tx, tt.box, other, 4*time.Hour)
	want := slices.Concat(tied, []uuid.UUID{newer, older, oldest})

	whole, err := readThreadMails(t.Context(), tt.tx, sdk.DefaultTenantID, thread, readPage, 0)
	page, pageErr := readThreadMails(t.Context(), tt.tx, sdk.DefaultTenantID, thread, 2, 2)

	if err != nil || !slices.Equal(mailIDs(whole), want) {
		t.Errorf("readThreadMails() = %v, %v, want %v", mailIDs(whole), err, want)
	}
	if pageErr != nil || !slices.Equal(mailIDs(page), want[2:4]) {
		t.Errorf("readThreadMails() of 2 after 2 = %v, %v, want %v", mailIDs(page), pageErr, want[2:4])
	}
}

func TestReadThreadMailAnswersAMailOfTheThreadAndNoneOfAnotherThread(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	asked := mailIn(t, tt.tx, tt.box, thread, 0)
	other := storeThread(t, tt.tx, tt.box, entryAt("Invoice", 0))
	elsewhere := mailIn(t, tt.tx, tt.box, other, 0)

	mail, found, err := readThreadMail(t.Context(), tt.tx, sdk.DefaultTenantID, thread, asked)

	if err != nil || !found || mail.id != asked || mail.threadID != thread {
		t.Errorf("readThreadMail() = %+v, %t, %v, want %v of the thread found", mail, found, err, asked)
	}
	for name, id := range map[string]uuid.UUID{
		"a mail of another thread": elsewhere, "an unknown id": uuid.Must(uuid.NewV7()),
	} {
		held, found, err := readThreadMail(t.Context(), tt.tx, sdk.DefaultTenantID, thread, id)

		if err != nil || found {
			t.Errorf("readThreadMail() of %s = %+v, %t, %v, want none found", name, held, found, err)
		}
	}
}

func TestReadContactMailsListsTheLinkedMailNewestFirstWithItsTotal(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	maria := tt.contacts["maria.perez@example.com"]
	quote := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	invoice := storeThread(t, tt.tx, tt.box, entryAt("Invoice", 2*time.Hour))
	first := mailIn(t, tt.tx, tt.box, quote, 0)
	latest := mailIn(t, tt.tx, tt.box, invoice, 2*time.Hour)
	middle := mailIn(t, tt.tx, tt.box, quote, time.Hour)
	for _, id := range []uuid.UUID{first, latest, middle} {
		linkMail(t, tt.tx, id, maria)
	}
	mailIn(t, tt.tx, tt.box, quote, 3*time.Hour)
	want := []uuid.UUID{latest, middle, first}

	whole, err := readContactMails(t.Context(), tt.tx, sdk.DefaultTenantID, maria, readPage, 0)
	page, pageErr := readContactMails(t.Context(), tt.tx, sdk.DefaultTenantID, maria, 1, 1)

	if err != nil || !slices.Equal(mailIDs(whole.items), want) || whole.total != 3 {
		t.Errorf("readContactMails() = %v of %d, %v, want %v of 3", mailIDs(whole.items), whole.total, err, want)
	}
	if pageErr != nil || !slices.Equal(mailIDs(page.items), want[1:2]) || page.total != 3 {
		t.Errorf("readContactMails() of 1 after 1 = %v of %d, %v, want %v of 3",
			mailIDs(page.items), page.total, pageErr, want[1:2])
	}
}

func TestReadContactMailsCountsOnlyTheContactsOwnMailInASharedThread(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	maria, ada := tt.contacts["maria.perez@example.com"], tt.contacts["ada@example.com"]
	thread := storeThread(t, tt.tx, tt.box, entryAt("Site visit on Friday", 0))
	copied := mailIn(t, tt.tx, tt.box, thread, 0)
	linkMail(t, tt.tx, copied, maria, ada)
	reply := mailIn(t, tt.tx, tt.box, thread, time.Hour)
	linkMail(t, tt.tx, reply, maria)

	for name, c := range map[string]struct {
		contact uuid.UUID
		want    []uuid.UUID
	}{
		"the contact in Cc":         {contact: ada, want: []uuid.UUID{copied}},
		"the contact on every mail": {contact: maria, want: []uuid.UUID{reply, copied}},
	} {
		page, err := readContactMails(t.Context(), tt.tx, sdk.DefaultTenantID, c.contact, readPage, 0)

		if err != nil || !slices.Equal(mailIDs(page.items), c.want) || page.total != len(c.want) {
			t.Errorf("readContactMails() of %s = %v of %d, %v, want %v of %d",
				name, mailIDs(page.items), page.total, err, c.want, len(c.want))
		}
	}
}

func TestTheLightReadsSelectNoBodyAndNoListOfPeople(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	id := mailIn(t, tt.tx, tt.box, thread, 0)
	maria := tt.contacts["maria.perez@example.com"]
	linkMail(t, tt.tx, id, maria)
	var sent []string
	db, ctx, tenant := recordingTx{Tx: tt.tx, sent: &sent}, t.Context(), sdk.DefaultTenantID

	for name, read := range map[string]func() error{
		"readInbox": func() error {
			_, err := readInbox(ctx, db, tenant, readPage, 0)
			return err
		},
		"readThread": func() error {
			_, _, err := readThread(ctx, db, tenant, thread)
			return err
		},
		"readThreadMails": func() error {
			_, err := readThreadMails(ctx, db, tenant, thread, readPage, 0)
			return err
		},
		"readThreadMail": func() error {
			_, _, err := readThreadMail(ctx, db, tenant, thread, id)
			return err
		},
		"readContactMails": func() error {
			_, err := readContactMails(ctx, db, tenant, maria, readPage, 0)
			return err
		},
	} {
		if err := read(); err != nil {
			t.Fatalf("%s() error = %v, want nil", name, err)
		}
	}

	if len(sent) != 7 {
		t.Fatalf("the light reads sent %d statements, want 7", len(sent))
	}
	for _, statement := range sent {
		if strings.Contains(statement, "body_text") || strings.Contains(statement, "safe_html") ||
			strings.Count(statement, "participants") != strings.Count(statement, firstSender) {
			t.Errorf("a light read sent %q, want no body and only the first sender of its people", statement)
		}
	}
}

func TestMigrateIndexesTheThreadsByTheirNewestMail(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	var definition string

	err := pool.QueryRow(t.Context(),
		"SELECT indexdef FROM pg_indexes WHERE schemaname = 'plugin_email' AND indexname = $1",
		"threads_last_message_idx").Scan(&definition)

	want := "ON plugin_email.threads USING btree (tenant_id, last_message_at DESC, id DESC)"
	if err != nil || !strings.HasSuffix(definition, want) {
		t.Errorf("the index threads_last_message_idx = %q, %v, want it to end %q", definition, err, want)
	}
}

func TestTheReadsReportAnEndedTransaction(t *testing.T) {
	t.Parallel()

	tx := endedTx(t)
	ctx, id, tenant := t.Context(), uuid.Must(uuid.NewV7()), sdk.DefaultTenantID
	for prefix, read := range map[string]func() error{
		"email: read the inbox: ": func() error {
			_, err := readInbox(ctx, tx, tenant, readPage, 0)
			return err
		},
		"email: read a thread: ": func() error {
			_, _, err := readThread(ctx, tx, tenant, id)
			return err
		},
		"email: read the mail of a thread: ": func() error {
			_, err := readThreadMails(ctx, tx, tenant, id, readPage, 0)
			return err
		},
		"email: read a mail of a thread: ": func() error {
			_, _, err := readThreadMail(ctx, tx, tenant, id, id)
			return err
		},
		"email: read the mail of a contact: ": func() error {
			_, err := readContactMails(ctx, tx, tenant, id, readPage, 0)
			return err
		},
	} {
		err := read()

		if !errors.Is(err, pgx.ErrTxClosed) || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("a read on an ended transaction error = %v, want %q naming %v", err, prefix, pgx.ErrTxClosed)
		}
	}
}

func TestTheReadsReportACountThatFails(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	ctx, id, tenant := t.Context(), uuid.Must(uuid.NewV7()), sdk.DefaultTenantID
	for prefix, c := range map[string]struct {
		fragment string
		read     func(querier) error
	}{
		"email: count the inbox: ": {fragment: "count(*) FROM plugin_email.threads", read: func(db querier) error {
			_, err := readInbox(ctx, db, tenant, readPage, 0)
			return err
		}},
		"email: count the mail of a contact: ": {
			fragment: "count(*) FROM plugin_email.message_contacts", read: func(db querier) error {
				_, err := readContactMails(ctx, db, tenant, id, readPage, 0)
				return err
			},
		},
	} {
		err := c.read(failingTx{Tx: beginTx(t, pool), fragment: c.fragment})

		if pgCode(err) != undefinedFunction || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("a read whose count fails error = %v, want %q naming the code %s", err, prefix, undefinedFunction)
		}
	}
}

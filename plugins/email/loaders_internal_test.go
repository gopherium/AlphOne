// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// testBatchWait is how long the loaders of a test gather keys.
const testBatchWait = 100 * time.Millisecond

// inRequest returns the context of one fresh graph request in the workspace tenant.
func inRequest(t *testing.T, tenant uuid.UUID) context.Context {
	t.Helper()
	return sdk.WithRequestScope(sdk.WithTenant(t.Context(), tenant), sdk.NewRequestScope())
}

// loadTogether loads each of ids through load at once, answering each value and error in the order of ids.
func loadTogether[T any](
	ctx context.Context, ids []uuid.UUID, load func(context.Context, uuid.UUID) (T, error),
) ([]T, []error) {
	values := make([]T, len(ids))
	errs := make([]error, len(ids))
	var asked sync.WaitGroup
	for i, id := range ids {
		asked.Go(func() { values[i], errs[i] = load(ctx, id) })
	}
	asked.Wait()
	return values, errs
}

// byIDAscending returns ids from the smallest to the largest.
func byIDAscending(ids ...uuid.UUID) []uuid.UUID {
	return slices.SortedFunc(slices.Values(ids), func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
}

// fileNames returns the name of each file of each list of files, in order.
func fileNames(lists [][]attachmentView) [][]string {
	names := make([][]string, len(lists))
	for i, files := range lists {
		names[i] = make([]string, 0, len(files))
		for _, file := range files {
			names[i] = append(names[i], file.filename)
		}
	}
	return names
}

// withoutIDs returns files with each id cleared, failing the test for a file the loader answered no id for.
func withoutIDs(t *testing.T, files []attachmentView) []attachmentView {
	t.Helper()
	cleared := make([]attachmentView, len(files))
	for i, file := range files {
		if file.id == uuid.Nil {
			t.Errorf("the file at %d answered no id, want the id of its row", file.position)
		}
		file.id = uuid.Nil
		cleared[i] = file
	}
	return cleared
}

// replyRow turns row into a reply from Ada Lovelace holding its own text.
func replyRow(row *messageRow) {
	row.participants = []participant{
		{Role: "from", Name: "Ada Lovelace", Email: "ada@example.com"}, {Role: "to", Email: "info@example.com"},
	}
	row.bodyText, row.safeHTML = "The lamp is on its way.", "<p>The lamp is on its way.</p>"
}

func TestEachLoaderReadsTheMailOfOneRequestInOneStatement(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	maria, ada := tt.contacts["maria.perez@example.com"], tt.contacts["ada@example.com"]
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	quote := mailIn(t, tt.tx, tt.box, thread, 0)
	reply := mailIn(t, tt.tx, tt.box, thread, time.Hour, replyRow)
	storeFiles(t, tt.tx, quote, fileAt(0, "quote.pdf", false))
	linkMail(t, tt.tx, quote, maria)
	linkMail(t, tt.tx, reply, maria, ada)
	var sent []string
	loaders := mailLoaders{
		db: recordingTx{Tx: tt.tx, sent: &sent}, bodiesPerRequest: 2, listsPerRequest: 2, batchWait: testBatchWait,
	}
	ids := []uuid.UUID{quote, reply}

	for name, c := range map[string]struct {
		load func(context.Context) (any, []error)
		want any
	}{
		"participants": {
			load: func(ctx context.Context) (any, []error) { return loadTogether(ctx, ids, loaders.participants) },
			want: [][]participant{
				{{Role: "from", Name: "Maria Perez", Email: "maria.perez@example.com"}},
				{{Role: "from", Name: "Ada Lovelace", Email: "ada@example.com"}, {Role: "to", Email: "info@example.com"}},
			},
		},
		"body": {
			load: func(ctx context.Context) (any, []error) { return loadTogether(ctx, ids, loaders.body) },
			want: []mailBody{
				{text: "Could you send me a quote?", html: "<p>Could you send me a quote?</p>"},
				{text: "The lamp is on its way.", html: "<p>The lamp is on its way.</p>"},
			},
		},
		"attachments": {
			load: func(ctx context.Context) (any, []error) {
				files, errs := loadTogether(ctx, ids, loaders.attachments)
				return fileNames(files), errs
			},
			want: [][]string{{"quote.pdf"}, {}},
		},
		"contactIDs": {
			load: func(ctx context.Context) (any, []error) { return loadTogether(ctx, ids, loaders.contactIDs) },
			want: [][]uuid.UUID{{maria}, byIDAscending(maria, ada)},
		},
	} {
		before := len(sent)

		got, errs := c.load(inRequest(t, sdk.DefaultTenantID))

		if errors.Join(errs...) != nil || !reflect.DeepEqual(got, c.want) || len(sent)-before != 1 {
			t.Errorf("the %s loader answered %+v, %v in %d statements, want %+v in one",
				name, got, errs, len(sent)-before, c.want)
		}
	}
}

func TestTheAttachmentLoaderAnswersEachFileOfAMailInPositionOrder(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Lamp order", 0))
	id := mailIn(t, tt.tx, tt.box, thread, 0)
	big := fileAt(1, "big.bin", false)
	big.contentType, big.state, big.fileID = octetStream, fileTooLarge, uuid.Nil
	storeFiles(t, tt.tx, id, fileAt(2, "lamp.png", true), fileAt(0, "quote.pdf", false), big)
	loaders := mailLoaders{db: tt.tx, bodiesPerRequest: 2, listsPerRequest: 1}

	files, err := loaders.attachments(inRequest(t, sdk.DefaultTenantID), id)

	want := []attachmentView{
		{position: 0, filename: "quote.pdf", contentType: "application/pdf", size: 3000, state: fileStored},
		{position: 1, filename: "big.bin", contentType: octetStream, size: 3000, state: fileTooLarge},
		{
			position: 2, filename: "lamp.png", contentType: "image/png", size: 3000, inline: true,
			contentID: "lamp-photo@example.com", state: fileStored,
		},
	}
	if got := withoutIDs(t, files); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("attachments() = %+v, %v, want %+v", got, err, want)
	}
}

func TestTheLoadersAnswerNoRequestScopeOnAContextWithoutOne(t *testing.T) {
	t.Parallel()

	loaders := mailLoaders{bodiesPerRequest: 2}
	ctx, id := t.Context(), uuid.Must(uuid.NewV7())

	for name, load := range map[string]func() error{
		"participants": func() error {
			_, err := loaders.participants(ctx, id)
			return err
		},
		"body": func() error {
			_, err := loaders.body(ctx, id)
			return err
		},
		"attachments": func() error {
			_, err := loaders.attachments(ctx, id)
			return err
		},
		"contactIDs": func() error {
			_, err := loaders.contactIDs(ctx, id)
			return err
		},
	} {
		if err := load(); !errors.Is(err, sdk.ErrNoRequestScope) {
			t.Errorf("%s() with no request scope error = %v, want %v", name, err, sdk.ErrNoRequestScope)
		}
	}
}

func TestTheLoadersAnswerTheFailureOfTheirReadForEveryMail(t *testing.T) {
	t.Parallel()

	loaders := mailLoaders{db: endedTx(t), bodiesPerRequest: 2, listsPerRequest: 2, batchWait: testBatchWait}
	ids := []uuid.UUID{uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())}

	for prefix, load := range map[string]func(context.Context) []error{
		"email: load the people of mail: ": func(ctx context.Context) []error {
			_, errs := loadTogether(ctx, ids, loaders.participants)
			return errs
		},
		"email: load the bodies of mail: ": func(ctx context.Context) []error {
			_, errs := loadTogether(ctx, ids, loaders.body)
			return errs
		},
		"email: load the files of mail: ": func(ctx context.Context) []error {
			_, errs := loadTogether(ctx, ids, loaders.attachments)
			return errs
		},
		"email: load the contacts of mail: ": func(ctx context.Context) []error {
			_, errs := loadTogether(ctx, ids, loaders.contactIDs)
			return errs
		},
	} {
		for i, err := range load(inRequest(t, sdk.DefaultTenantID)) {
			if !errors.Is(err, pgx.ErrTxClosed) || !strings.HasPrefix(err.Error(), prefix) {
				t.Errorf("a load of mail %d on an ended transaction error = %v, want %q naming %v",
					i, err, prefix, pgx.ErrTxClosed)
			}
		}
	}
}

func TestTheBodyLoaderReadsTheBodiesWithinTheBoundAndRejectsTheRest(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	quote := mailIn(t, tt.tx, tt.box, thread, 0)
	reply := mailIn(t, tt.tx, tt.box, thread, time.Hour, replyRow)
	thanks := mailIn(t, tt.tx, tt.box, thread, 2*time.Hour, func(row *messageRow) {
		row.bodyText, row.safeHTML = "Thank you for the lamp.", "<p>Thank you for the lamp.</p>"
	})
	loaders := mailLoaders{db: tt.tx, bodiesPerRequest: 2, listsPerRequest: 3}
	ctx := inRequest(t, sdk.DefaultTenantID)
	for _, id := range []uuid.UUID{quote, reply, thanks} {
		if _, err := loaders.participants(ctx, id); err != nil {
			t.Fatalf("participants() error = %v, want nil", err)
		}
	}

	first, firstErr := loaders.body(ctx, quote)
	second, secondErr := loaders.body(ctx, reply)
	_, pastErr := loaders.body(ctx, thanks)
	_, againErr := loaders.body(ctx, quote)
	fresh, freshErr := loaders.body(inRequest(t, sdk.DefaultTenantID), thanks)

	if firstErr != nil || first.text != "Could you send me a quote?" || secondErr != nil ||
		second.html != "<p>The lamp is on its way.</p>" {
		t.Errorf("the bodies within the bound = %+v, %v and %+v, %v, want both read", first, firstErr, second, secondErr)
	}
	for name, err := range map[string]error{"a third mail": pastErr, "the first mail asked again": againErr} {
		var rejected sdk.GraphError
		if !errors.As(err, &rejected) || rejected.Code != "VALIDATION" ||
			rejected.Reason != "bodies_per_request_exceeded" || !reflect.DeepEqual(rejected.Meta, map[string]any{"max": 2}) ||
			!errors.Is(err, errTooManyBodies) {
			t.Errorf("the body of %s past the bound error = %#v, want bodies_per_request_exceeded with the max 2", name, err)
		}
	}
	if freshErr != nil || fresh.text != "Thank you for the lamp." {
		t.Errorf("the body of a new request = %+v, %v, want it read", fresh, freshErr)
	}
}

func TestThePeopleAndFileLoadersShareOneBoundAndRejectTheListsPastIt(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	thread := storeThread(t, tt.tx, tt.box, entryAt("Your quote", 0))
	quote := mailIn(t, tt.tx, tt.box, thread, 0)
	reply := mailIn(t, tt.tx, tt.box, thread, time.Hour, replyRow)
	thanks := mailIn(t, tt.tx, tt.box, thread, 2*time.Hour)
	storeFiles(t, tt.tx, quote, fileAt(0, "quote.pdf", false))
	var sent []string
	loaders := mailLoaders{db: recordingTx{Tx: tt.tx, sent: &sent}, bodiesPerRequest: 2, listsPerRequest: 3}
	ctx := inRequest(t, sdk.DefaultTenantID)
	for _, id := range []uuid.UUID{quote, reply, thanks, quote} {
		if _, err := loaders.contactIDs(ctx, id); err != nil {
			t.Fatalf("contactIDs() error = %v, want nil", err)
		}
	}
	for _, id := range []uuid.UUID{quote, reply} {
		if _, err := loaders.body(ctx, id); err != nil {
			t.Fatalf("body() error = %v, want nil", err)
		}
	}

	people, peopleErr := loaders.participants(ctx, quote)
	files, filesErr := loaders.attachments(ctx, quote)
	again, againErr := loaders.participants(ctx, reply)
	before := len(sent)
	_, pastFilesErr := loaders.attachments(ctx, reply)
	_, pastPeopleErr := loaders.participants(ctx, thanks)
	pastSent := len(sent) - before
	fresh, freshErr := loaders.attachments(inRequest(t, sdk.DefaultTenantID), quote)

	if peopleErr != nil || len(people) != 1 || people[0].Email != "maria.perez@example.com" ||
		filesErr != nil || !reflect.DeepEqual(fileNames([][]attachmentView{files}), [][]string{{"quote.pdf"}}) ||
		againErr != nil || len(again) != 2 || again[0].Email != "ada@example.com" {
		t.Errorf("the lists within the bound = %+v, %v and %+v, %v and %+v, %v, want the people and files of both mails",
			people, peopleErr, files, filesErr, again, againErr)
	}
	for name, err := range map[string]error{
		"the files of the reply": pastFilesErr, "the people of a third mail": pastPeopleErr,
	} {
		var rejected sdk.GraphError
		if !errors.As(err, &rejected) || rejected.Code != "VALIDATION" ||
			rejected.Reason != "lists_per_request_exceeded" || !reflect.DeepEqual(rejected.Meta, map[string]any{"max": 3}) ||
			!errors.Is(err, errTooManyLists) {
			t.Errorf("%s past the bound error = %#v, want lists_per_request_exceeded with the max 3", name, err)
		}
	}
	if pastSent != 0 {
		t.Errorf("the lists past the bound sent %d statements, want none", pastSent)
	}
	if freshErr != nil || !reflect.DeepEqual(fileNames([][]attachmentView{fresh}), [][]string{{"quote.pdf"}}) {
		t.Errorf("the files of a new request = %+v, %v, want quote.pdf read", fresh, freshErr)
	}
}

func TestTheLoadersKeepToTheirWorkspace(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	acme, box := it.addWorkspace()
	it.ingestIn(sdk.DefaultTenantID, it.box, invoiceMail("%PDF-1.4 invoice"), "file:invoice")
	it.ingestIn(acme, box, invoiceMail("%PDF-1.4 invoice"), "file:invoice")
	home, own := soleInboxMail(t, it.pool, sdk.DefaultTenantID), soleInboxMail(t, it.pool, acme)
	loaders := mailLoaders{db: it.pool, bodiesPerRequest: 2, listsPerRequest: 2}
	inAcme := inRequest(t, acme)

	_, peopleErr := loaders.participants(inAcme, home)
	_, bodyErr := loaders.body(inAcme, home)
	files, filesErr := loaders.attachments(inAcme, home)
	contacts, contactsErr := loaders.contactIDs(inAcme, home)

	if !errors.Is(peopleErr, errMailNotHeld) || !errors.Is(bodyErr, errMailNotHeld) {
		t.Errorf("the people and body of the default mail in Acme error = %v and %v, want %v",
			peopleErr, bodyErr, errMailNotHeld)
	}
	if len(files) != 0 || filesErr != nil || len(contacts) != 0 || contactsErr != nil {
		t.Errorf("the files and contacts of the default mail in Acme = %+v, %v and %v, %v, want none",
			files, filesErr, contacts, contactsErr)
	}
	ownCtx := inRequest(t, acme)
	people, peopleErr := loaders.participants(ownCtx, own)
	body, bodyErr := loaders.body(ownCtx, own)
	files, filesErr = loaders.attachments(ownCtx, own)
	contacts, contactsErr = loaders.contactIDs(ownCtx, own)
	maria := it.matcher.known[acme]["maria.perez@example.com"]
	if peopleErr != nil || len(people) == 0 || people[0].Email != "maria.perez@example.com" ||
		bodyErr != nil || body.text != "Here is the invoice." {
		t.Errorf("the people and body of Acme's mail = %+v, %v and %+v, %v, want its own", people, peopleErr, body, bodyErr)
	}
	got := fileNames([][]attachmentView{files})
	if filesErr != nil || !reflect.DeepEqual(got, [][]string{{"invoice.pdf"}}) ||
		contactsErr != nil || !slices.Equal(contacts, []uuid.UUID{maria}) {
		t.Errorf("the files and contacts of Acme's mail = %v, %v and %v, %v, want invoice.pdf and %v",
			got, filesErr, contacts, contactsErr, maria)
	}
}

// soleInboxMail returns the newest mail of the only thread the workspace tenant holds.
func soleInboxMail(t *testing.T, db querier, tenant uuid.UUID) uuid.UUID {
	t.Helper()
	page, err := readInbox(t.Context(), db, tenant, readPage, 0)
	if err != nil || len(page.items) != 1 {
		t.Fatalf("readInbox() = %+v, %v, want one thread", page, err)
	}
	return page.items[0].latest.id
}

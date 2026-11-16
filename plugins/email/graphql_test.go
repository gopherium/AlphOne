// SPDX-License-Identifier: Elastic-2.0

package email_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/google/uuid"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/credential"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// inertDatabaseURL names a database the plugins the graph holds beside the wired one never reach.
const inertDatabaseURL = "postgres://graph:graph@localhost:1/graph"

// emailRead is the scope a token reading mail holds.
const emailRead = "email:read"

// filesMail is Maria Perez asking for a quote with a short note and a plan, sent in copy to Ada Lovelace.
const filesMail = "From: Maria Perez <maria.perez@example.com>\r\nTo: info@example.com\r\n" +
	"Cc: Ada Lovelace <ada@example.com>\r\nSubject: Lamp quote\r\nDate: Mon, 02 Nov 2026 09:00:00 +0000\r\n" +
	"Message-ID: <lamp-quote@example.com>\r\nMIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=\"files\"\r\n\r\n" +
	"--files\r\nContent-Type: text/plain\r\n\r\nCould you send me a quote for the new lamp?\r\n" +
	"--files\r\nContent-Type: text/plain; name=\"note.txt\"\r\n" +
	"Content-Disposition: attachment; filename=\"note.txt\"\r\n\r\nBrass\r\n" +
	"--files\r\nContent-Type: application/pdf; name=\"plan.pdf\"\r\n" +
	"Content-Disposition: attachment; filename=\"plan.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\nJVBERi0xLjQKJcOkw7zDtsOfCg==\r\n" +
	"--files--\r\n"

// filesCap is the attachment cap under which the note of filesMail is kept and its plan is not.
const filesCap = "8"

// noteBytes is the size of the note filesMail carries.
const noteBytes = 5

// graphAnswer is one answer of the graph, its data kept raw by root field.
type graphAnswer struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []graphFault               `json:"errors"`
}

// graphFault is one error the graph answers.
type graphFault struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path"`
	Extensions map[string]any `json:"extensions"`
}

// servedPage is one page of threads or mail as the graph answers it.
type servedPage[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
	Limit int `json:"limit"`
}

// servedThread is a thread as the graph answers it.
type servedThread struct {
	ID             string       `json:"id"`
	Subject        string       `json:"subject"`
	FirstMessageAt string       `json:"firstMessageAt"`
	LastMessageAt  string       `json:"lastMessageAt"`
	MessageCount   int          `json:"messageCount"`
	HasAttachments bool         `json:"hasAttachments"`
	Latest         *servedMail  `json:"latest"`
	Messages       []servedMail `json:"messages"`
	Message        *servedMail  `json:"message"`
	Unknown        *servedMail  `json:"unknown"`
	Elsewhere      *servedMail  `json:"elsewhere"`
}

// servedMail is a mail as the graph answers it.
type servedMail struct {
	ID           string             `json:"id"`
	ThreadID     string             `json:"threadId"`
	Direction    string             `json:"direction"`
	Subject      string             `json:"subject"`
	From         *servedPerson      `json:"from"`
	SentAt       *string            `json:"sentAt"`
	ReceivedAt   string             `json:"receivedAt"`
	Snippet      string             `json:"snippet"`
	BodyState    string             `json:"bodyState"`
	SkippedParts int                `json:"skippedParts"`
	Automation   string             `json:"automation"`
	Encrypted    bool               `json:"encrypted"`
	Signed       bool               `json:"signed"`
	Winmail      bool               `json:"winmail"`
	InviteMethod string             `json:"inviteMethod"`
	RawState     string             `json:"rawState"`
	RawSize      float64            `json:"rawSize"`
	OriginalPath *string            `json:"originalPath"`
	Participants []servedPerson     `json:"participants"`
	Body         *servedBody        `json:"body"`
	Attachments  []servedAttachment `json:"attachments"`
	ContactIDs   []string           `json:"contactIds"`
}

// servedPerson is a person a mail names, as the graph answers it.
type servedPerson struct {
	Role  string `json:"role"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// servedBody is the text and the safe HTML of a mail, as the graph answers them.
type servedBody struct {
	Text string `json:"text"`
	HTML string `json:"html"`
}

// servedAttachment is a file of a mail, as the graph answers it.
type servedAttachment struct {
	ID           string  `json:"id"`
	Position     int     `json:"position"`
	Filename     string  `json:"filename"`
	ContentType  string  `json:"contentType"`
	Size         float64 `json:"size"`
	Inline       bool    `json:"inline"`
	ContentID    string  `json:"contentId"`
	State        string  `json:"state"`
	DownloadPath *string `json:"downloadPath"`
}

// servedMailbox is a mailbox as the graph answers it.
type servedMailbox struct {
	ID      string   `json:"id"`
	Address string   `json:"address"`
	Aliases []string `json:"aliases"`
}

// reader asks the graph as a token holding scopes, inside a request scope unless unscoped.
type reader struct {
	graph    http.Handler
	scopes   string
	unscoped bool
}

// newEmailGraph returns the composed graph over the wired plugin, priced and gated as the host serves it.
func newEmailGraph(t *testing.T, w wiredPlugin) (http.Handler, graphql.ExecutableSchema) {
	t.Helper()
	inert, err := graphroot.All(sdk.Deps{DatabaseURL: inertDatabaseURL})
	if err != nil {
		t.Fatalf("graphroot.All() error = %v, want nil", err)
	}
	for _, registered := range inert {
		t.Cleanup(func() { _ = registered.Stop(context.Background()) })
	}
	root, err := graphroot.FromPlugins(&graphres.Resolver{}, append(inert, w.plugin))
	if err != nil {
		t.Fatalf("FromPlugins() error = %v, want nil", err)
	}
	schema := graphres.ExecutableSchema(root)
	srv := handler.New(schema)
	srv.AddTransport(transport.POST{})
	srv.Use(&graphres.CostLimit{})
	srv.Use(&graphres.RepeatedFields{})
	srv.AroundOperations(graphres.ScopeGate(graphres.NewScopeMap(schema.Schema()), role.NewRegistry()))
	srv.SetErrorPresenter(graphres.PresentError)
	return srv, schema
}

// newReader returns a reader of the graph over the wired plugin as a token holding scopes.
func newReader(t *testing.T, w wiredPlugin, scopes string) reader {
	t.Helper()
	graph, _ := newEmailGraph(t, w)
	return reader{graph: graph, scopes: scopes}
}

// ask answers what the graph replies to query with variables.
func (r reader) ask(t *testing.T, query string, variables map[string]any) graphAnswer {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		t.Fatalf("encoding the query: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ctx := credential.WithToken(request.Context(), credential.Token{
		Name: "reader", Scopes: apitoken.Scopes(strings.Fields(r.scopes)),
	})
	if !r.unscoped {
		ctx = sdk.WithRequestScope(ctx, sdk.NewRequestScope())
	}
	recorder := httptest.NewRecorder()
	r.graph.ServeHTTP(recorder, request.WithContext(ctx))
	var answer graphAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the graph answered %d with %q: %v", recorder.Code, recorder.Body.String(), err)
	}
	return answer
}

// read decodes into dest the root field called name of what the graph replies to query, failing on any error.
func (r reader) read(t *testing.T, query string, variables map[string]any, name string, dest any) {
	t.Helper()
	answer := r.ask(t, query, variables)
	if len(answer.Errors) > 0 {
		t.Fatalf("the graph answered the errors %+v to %s, want none", answer.Errors, query)
	}
	if err := json.Unmarshal(answer.Data[name], dest); err != nil {
		t.Fatalf("reading %s from %s: %v", name, answer.Data[name], err)
	}
}

// reasons answers the reason of each error of the answer.
func (a graphAnswer) reasons() []any {
	reasons := make([]any, len(a.Errors))
	for i, fault := range a.Errors {
		reasons[i] = fault.Extensions["reason"]
	}
	return reasons
}

// feed stores raw in the default workspace through email:ingest with flags beside the mailbox ones, or fails.
func (w wiredPlugin) feed(t *testing.T, raw string, flags map[string]string) {
	t.Helper()
	held := map[string]string{"mailbox": "info@example.com", "alias": "sales@example.com"}
	maps.Copy(held, flags)
	var answered bytes.Buffer
	err := w.plugin.Commands()[0].Run(t.Context(), sdk.Call{
		Args: []string{sdk.DefaultTenantID.String()}, Flags: held,
		Stdin: strings.NewReader(raw), Stdout: &answered, Stderr: io.Discard, Apply: true,
	})
	if err != nil || answered.String() != "stored\n" {
		t.Fatalf("email:ingest = %q, %v, want stored", answered.String(), err)
	}
}

// receivedAt feeds raw as received at the clock time of 2 November 2026, or fails.
func (w wiredPlugin) receivedAt(t *testing.T, clock, raw string) {
	t.Helper()
	w.feed(t, raw, map[string]string{"received": "2026-11-02T" + clock + ":00Z"})
}

// contactID reads the id of the known contact at email in the default workspace.
func (w wiredPlugin) contactID(t *testing.T, email string) string {
	t.Helper()
	var id uuid.UUID
	if err := w.pool.QueryRow(t.Context(), "SELECT id FROM core.contacts WHERE tenant_id = $1 AND name = $2",
		sdk.DefaultTenantID, email).Scan(&id); err != nil {
		t.Fatalf("reading the contact %s: %v", email, err)
	}
	return id.String()
}

// plainMail is a plain text mail from sender with subject and id, a reply to replyTo unless it is empty.
func plainMail(sender, subject, id, replyTo, text string) string {
	headers := "From: " + sender + "\r\nTo: info@example.com\r\nSubject: " + subject +
		"\r\nDate: Mon, 02 Nov 2026 09:00:00 +0000\r\nMessage-ID: <" + id + ">\r\n"
	if replyTo != "" {
		headers += "In-Reply-To: <" + replyTo + ">\r\nReferences: <" + replyTo + ">\r\n"
	}
	return headers + "Content-Type: text/plain\r\n\r\n" + text + "\r\n"
}

// mariaPerez is the sender line of Maria Perez.
const mariaPerez = "Maria Perez <maria.perez@example.com>"

// adaLovelace is the sender line of Ada Lovelace.
const adaLovelace = "Ada Lovelace <ada@example.com>"

// inboxQuery reads one page of the inbox with the newest mail of each thread.
const inboxQuery = `query Inbox($limit: Int, $offset: Int) {
	emailThreads(limit: $limit, offset: $offset) {
		total limit
		items {
			id subject firstMessageAt lastMessageAt messageCount hasAttachments
			latest { id subject from { name email } snippet }
		}
	}
}`

// threadQuery reads one thread with one page of its mail.
const threadQuery = `query Thread($id: UUID!, $limit: Int, $offset: Int) {
	emailThread(id: $id) { id subject messageCount messages(limit: $limit, offset: $offset) { id subject } }
}`

// mailQuery reads one mail of a thread with every field it answers.
const mailQuery = `query Mail($thread: UUID!, $id: UUID!) {
	emailThread(id: $thread) {
		message(id: $id) {
			id threadId direction subject from { name email } sentAt receivedAt snippet bodyState skippedParts
			automation encrypted signed winmail inviteMethod rawState rawSize originalPath
			participants { role name email }
			body { text html }
			attachments { id position filename contentType size inline contentId state downloadPath }
			contactIds
		}
	}
}`

// contactMailQuery reads the mail linked to one contact, newest first.
const contactMailQuery = `query ContactMail($contact: UUID!, $limit: Int, $offset: Int) {
	emailContactMessages(contactId: $contact, limit: $limit, offset: $offset) {
		total limit items { id threadId subject }
	}
}`

// screenInboxQuery is the inbox a screen asks for, a page of the default size with the newest mail of each thread.
const screenInboxQuery = `{ emailThreads { total limit items {
	id subject firstMessageAt lastMessageAt messageCount hasAttachments
	latest { id threadId subject from { name email } receivedAt snippet bodyState rawState }
} } }`

// soleThread answers the one thread the inbox of r lists.
func soleThread(t *testing.T, r reader) servedThread {
	t.Helper()
	var page servedPage[servedThread]
	r.read(t, inboxQuery, nil, "emailThreads", &page)
	if len(page.Items) != 1 {
		t.Fatalf("the inbox lists %+v, want one thread", page.Items)
	}
	return page.Items[0]
}

// openMail answers the mail id of the thread named thread with every field it answers.
func openMail(t *testing.T, r reader, thread, id string) servedMail {
	t.Helper()
	var found *servedThread
	r.read(t, mailQuery, map[string]any{"thread": thread, "id": id}, "emailThread", &found)
	if found == nil || found.Message == nil {
		t.Fatalf("the thread %s answered %+v, want the mail %s", thread, found, id)
	}
	return *found.Message
}

// subjectsOf answers the subjects of mails in order.
func subjectsOf(mails []servedMail) []string {
	subjects := make([]string, len(mails))
	for i, mail := range mails {
		subjects[i] = mail.Subject
	}
	return subjects
}

func TestTheMailboxAnswersNoneBeforeAnyMailThenItsAddressAndAliases(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	r := newReader(t, w, emailRead)
	var none *servedMailbox
	r.read(t, `{ emailMailbox { id address aliases } }`, nil, "emailMailbox", &none)
	w.receivedAt(t, "09:00", plainMail(mariaPerez, "Lamp quote", "lamp-quote@example.com", "", "Hello."))

	var box *servedMailbox
	r.read(t, `{ emailMailbox { id address aliases } }`, nil, "emailMailbox", &box)

	if none != nil {
		t.Errorf("emailMailbox before any mail = %+v, want null", *none)
	}
	if box == nil || uuid.Validate(box.ID) != nil || box.Address != "info@example.com" ||
		!slices.Equal(box.Aliases, []string{"sales@example.com"}) {
		t.Errorf("emailMailbox = %+v, want info@example.com with the alias sales@example.com", box)
	}
}

func TestTheInboxListsThreadsByTheirNewestMailWithItsTotalAndPage(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	w.receivedAt(t, "09:00", plainMail(mariaPerez, "Opening hours", "opening-hours@example.com", "", "When?"))
	w.receivedAt(t, "09:05", filesMail)
	w.receivedAt(t, "09:10", plainMail(adaLovelace, "Delivery date", "delivery-date@example.com", "", "Soon?"))
	w.receivedAt(t, "09:15", plainMail(mariaPerez, "Re: Opening hours", "opening-hours-again@example.com",
		"opening-hours@example.com", "And on Sunday?"))
	r := newReader(t, w, emailRead)

	var whole, page servedPage[servedThread]
	r.read(t, inboxQuery, nil, "emailThreads", &whole)
	r.read(t, inboxQuery, map[string]any{"limit": 2, "offset": 1}, "emailThreads", &page)

	if whole.Total != 3 || whole.Limit != sdk.DefaultPageSize || len(whole.Items) != 3 {
		t.Fatalf("the inbox = %+v, want 3 threads on a page of %d", whole, sdk.DefaultPageSize)
	}
	opening := whole.Items[0]
	if opening.Subject != "Opening hours" || opening.MessageCount != 2 || opening.HasAttachments ||
		opening.FirstMessageAt != "2026-11-02T09:00:00Z" || opening.LastMessageAt != "2026-11-02T09:15:00Z" {
		t.Errorf("the newest thread = %+v, want Opening hours with 2 mails from 09:00 to 09:15 and no files", opening)
	}
	if latest := opening.Latest; latest == nil || latest.Subject != "Re: Opening hours" || latest.From == nil ||
		*latest.From != (servedPerson{Name: "Maria Perez", Email: "maria.perez@example.com"}) ||
		latest.Snippet != "And on Sunday?" {
		t.Errorf("the newest mail of Opening hours = %+v, want the reply of Maria Perez", latest)
	}
	if page.Total != 3 || page.Limit != 2 || len(page.Items) != 2 ||
		page.Items[0].Subject != "Delivery date" || page.Items[1].Subject != "Lamp quote" ||
		!page.Items[1].HasAttachments {
		t.Errorf("the second page = %+v, want Delivery date then Lamp quote with its files, of 3 on pages of 2", page)
	}
}

func TestAThreadListsItsMailNewestFirstAndOpensOneMailWithEveryField(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, map[string]string{"ALPHONE_EMAIL_ATTACHMENT_MAX_BYTES": filesCap})
	w.receivedAt(t, "09:00", filesMail)
	w.receivedAt(t, "09:30", plainMail(adaLovelace, "Re: Lamp quote", "lamp-quote-reply@example.com",
		"lamp-quote@example.com", "The brass one is lovely."))
	r := newReader(t, w, emailRead)
	thread := soleThread(t, r)

	var listed, older *servedThread
	r.read(t, threadQuery, map[string]any{"id": thread.ID}, "emailThread", &listed)
	r.read(t, threadQuery, map[string]any{"id": thread.ID, "limit": 1, "offset": 1}, "emailThread", &older)
	quote := openMail(t, r, thread.ID, older.Messages[0].ID)

	if got := subjectsOf(listed.Messages); listed.MessageCount != 2 ||
		!slices.Equal(got, []string{"Re: Lamp quote", "Lamp quote"}) {
		t.Errorf("the thread lists %q of %d mails, want the reply then the quote", got, listed.MessageCount)
	}
	if got := subjectsOf(older.Messages); !slices.Equal(got, []string{"Lamp quote"}) {
		t.Errorf("the thread past its newest mail lists %q, want the quote alone", got)
	}
	wantPath := "/api/plugins/email/messages/" + quote.ID + "/original"
	if quote.ThreadID != thread.ID || quote.Direction != "inbound" || quote.Subject != "Lamp quote" ||
		quote.From == nil || quote.From.Email != "maria.perez@example.com" || quote.SentAt == nil ||
		*quote.SentAt != "2026-11-02T09:00:00Z" || quote.ReceivedAt != "2026-11-02T09:00:00Z" ||
		quote.BodyState != "whole" || quote.SkippedParts != 0 || quote.Automation != "" || quote.Encrypted ||
		quote.Signed || quote.Winmail || quote.InviteMethod != "" || quote.RawState != "stored" ||
		quote.OriginalPath == nil || *quote.OriginalPath != wantPath {
		t.Errorf("the quote = %+v, want Maria Perez's whole mail of 09:00 kept with its original at %s", quote, wantPath)
	}
	wantPeople := []servedPerson{
		{Role: "from", Name: "Maria Perez", Email: "maria.perez@example.com"},
		{Role: "to", Email: "info@example.com"},
		{Role: "cc", Name: "Ada Lovelace", Email: "ada@example.com"},
	}
	if !slices.Equal(quote.Participants, wantPeople) {
		t.Errorf("the quote names %+v, want %+v", quote.Participants, wantPeople)
	}
	if quote.Body == nil || strings.TrimSpace(quote.Body.Text) != "Could you send me a quote for the new lamp?" {
		t.Errorf("the quote reads %+v, want Maria Perez's question", quote.Body)
	}
	wantContacts := []string{w.contactID(t, "ada@example.com"), w.contactID(t, "maria.perez@example.com")}
	slices.Sort(wantContacts)
	if !slices.Equal(quote.ContactIDs, wantContacts) {
		t.Errorf("the quote is linked to %q, want %q", quote.ContactIDs, wantContacts)
	}
}

func TestAMailThatNamesNoSenderAnswersNoFrom(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	w.receivedAt(t, "09:00", "To: info@example.com\r\nCc: Maria Perez <maria.perez@example.com>\r\n"+
		"Subject: Unsigned note\r\nDate: Mon, 02 Nov 2026 09:00:00 +0000\r\nMessage-ID: <unsigned-note@example.com>\r\n"+
		"Content-Type: text/plain\r\n\r\nThe lamp arrived.\r\n")
	r := newReader(t, w, emailRead)

	thread := soleThread(t, r)

	if thread.Latest == nil || thread.Latest.Subject != "Unsigned note" || thread.Latest.From != nil {
		t.Errorf("the newest mail = %+v, want the unsigned note with no sender", thread.Latest)
	}
}

func TestAThreadAndAMailOfAnotherThreadOrUnknownAnswerNothing(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	w.receivedAt(t, "09:00", plainMail(mariaPerez, "Lamp quote", "lamp-quote@example.com", "", "Hello."))
	w.receivedAt(t, "09:05", plainMail(adaLovelace, "Delivery date", "delivery-date@example.com", "", "Soon?"))
	r := newReader(t, w, emailRead)
	var page servedPage[servedThread]
	r.read(t, inboxQuery, nil, "emailThreads", &page)
	delivery, quote := page.Items[0], page.Items[1]

	var unknown, opened *servedThread
	r.read(t, threadQuery, map[string]any{"id": uuid.NewString()}, "emailThread", &unknown)
	r.read(t, `query Mails($thread: UUID!, $other: UUID!, $unknown: UUID!) {
		emailThread(id: $thread) { elsewhere: message(id: $other) { id } unknown: message(id: $unknown) { id } }
	}`, map[string]any{"thread": quote.ID, "other": delivery.Latest.ID, "unknown": uuid.NewString()},
		"emailThread", &opened)

	if unknown != nil {
		t.Errorf("emailThread of an unknown id = %+v, want null", *unknown)
	}
	if opened == nil || opened.Elsewhere != nil || opened.Unknown != nil {
		t.Errorf("the quote's thread answered %+v, want null for a mail of another thread and an unknown one", opened)
	}
}

func TestAContactsMailListsOnlyTheMailLinkedToIt(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	w.receivedAt(t, "09:00", filesMail)
	w.receivedAt(t, "09:30", plainMail(adaLovelace, "Re: Lamp quote", "lamp-quote-reply@example.com",
		"lamp-quote@example.com", "The brass one is lovely."))
	r := newReader(t, w, emailRead)
	thread := soleThread(t, r)

	var maria, ada, adaOlder servedPage[servedMail]
	r.read(t, contactMailQuery, map[string]any{"contact": w.contactID(t, "maria.perez@example.com")},
		"emailContactMessages", &maria)
	r.read(t, contactMailQuery, map[string]any{"contact": w.contactID(t, "ada@example.com")},
		"emailContactMessages", &ada)
	r.read(t, contactMailQuery, map[string]any{"contact": w.contactID(t, "ada@example.com"), "limit": 1, "offset": 1},
		"emailContactMessages", &adaOlder)

	if got := subjectsOf(maria.Items); maria.Total != 1 || !slices.Equal(got, []string{"Lamp quote"}) ||
		maria.Items[0].ThreadID != thread.ID {
		t.Errorf("Maria Perez's mail = %+v, want the quote alone in its thread", maria)
	}
	if got := subjectsOf(ada.Items); ada.Total != 2 || ada.Limit != sdk.DefaultPageSize ||
		!slices.Equal(got, []string{"Re: Lamp quote", "Lamp quote"}) {
		t.Errorf("Ada Lovelace's mail = %+v, want the reply then the quote of 2", ada)
	}
	if got := subjectsOf(adaOlder.Items); adaOlder.Total != 2 || adaOlder.Limit != 1 ||
		!slices.Equal(got, []string{"Lamp quote"}) {
		t.Errorf("Ada Lovelace's mail past her newest = %+v, want the quote of 2 on pages of 1", adaOlder)
	}
}

func TestEveryEmailRootFieldNeedsTheEmailArea(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	contacts, mail := newReader(t, w, "contacts:read"), newReader(t, w, emailRead)
	contact := w.contactID(t, "maria.perez@example.com")

	for _, query := range []string{
		`{ emailMailbox { id } }`,
		`{ emailThreads { total } }`,
		fmt.Sprintf(`{ emailThread(id: %q) { id } }`, uuid.NewString()),
		fmt.Sprintf(`{ emailContactMessages(contactId: %q) { total } }`, contact),
	} {
		rejected, answered := contacts.ask(t, query, nil), mail.ask(t, query, nil)

		if len(rejected.Errors) != 1 || rejected.Errors[0].Message != "scope required: email:read" {
			t.Errorf("%s with contacts:read answered %+v, want it rejected for want of email:read", query, rejected.Errors)
		}
		if len(answered.Errors) != 0 {
			t.Errorf("%s with email:read answered %+v, want no error", query, answered.Errors)
		}
	}
}

func TestAnEmailListPastItsBoundsIsRejected(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	w.receivedAt(t, "09:00", plainMail(mariaPerez, "Lamp quote", "lamp-quote@example.com", "", "Hello."))
	r := newReader(t, w, emailRead)
	thread, contact := soleThread(t, r).ID, w.contactID(t, "maria.perez@example.com")

	for _, limit := range []int{0, sdk.DefaultPageCap + 1} {
		for _, query := range []string{
			fmt.Sprintf(`{ emailThreads(limit: %d) { total } }`, limit),
			fmt.Sprintf(`{ emailThread(id: %q) { messages(limit: %d) { id } } }`, thread, limit),
			fmt.Sprintf(`{ emailContactMessages(contactId: %q, limit: %d) { total } }`, contact, limit),
		} {
			answer := r.ask(t, query, nil)

			if !slices.Equal(answer.reasons(), []any{"first_out_of_range"}) {
				t.Errorf("%s answered %+v, want it rejected as first_out_of_range", query, answer.Errors)
			}
		}
	}
}

func TestTheInboxAScreenAsksForCostsLessThanTheLimit(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	graph, schema := newEmailGraph(t, w)
	query := gqlparser.MustLoadQueryWithRules(schema.Schema(), screenInboxQuery, rules.NewDefaultRules())

	cost := complexity.Calculate(t.Context(), schema, query.Operations[0], nil)
	answer := reader{graph: graph, scopes: emailRead}.ask(t, screenInboxQuery, nil)

	if cost >= graphres.ComplexityLimit {
		t.Errorf("the inbox of a screen costs %d, want less than %d", cost, graphres.ComplexityLimit)
	}
	if len(answer.Errors) != 0 {
		t.Errorf("the inbox of a screen answered %+v, want no error", answer.Errors)
	}
}

func TestTheDownloadPathsNameOnlyAKeptFileAndTheByteCountsAreWhole(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, map[string]string{"ALPHONE_EMAIL_ATTACHMENT_MAX_BYTES": filesCap})
	w.receivedAt(t, "09:00", filesMail)
	r := newReader(t, w, emailRead)
	thread := soleThread(t, r)

	quote := openMail(t, r, thread.ID, thread.Latest.ID)

	if quote.RawSize != float64(len(filesMail)) {
		t.Errorf("the quote's raw size = %v, want the %d bytes fed", quote.RawSize, len(filesMail))
	}
	if len(quote.Attachments) != 2 {
		t.Fatalf("the quote holds the files %+v, want the note and the plan", quote.Attachments)
	}
	note, plan := quote.Attachments[0], quote.Attachments[1]
	wantNote := "/api/plugins/email/attachments/" + note.ID
	if note.Filename != "note.txt" || note.State != "stored" || note.Size != noteBytes || note.Position != 0 ||
		note.Inline || note.DownloadPath == nil || *note.DownloadPath != wantNote {
		t.Errorf("the note = %+v, want it kept with its %d bytes at %s", note, noteBytes, wantNote)
	}
	if plan.Filename != "plan.pdf" || plan.ContentType != "application/pdf" || plan.State != "too_large" ||
		plan.Size <= noteBytes || plan.Size != float64(int(plan.Size)) || plan.DownloadPath != nil {
		t.Errorf("the plan = %+v, want it too large with a whole byte count and no download", plan)
	}
}

func TestAnOriginalNotKeptOffersNoPath(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, map[string]string{"ALPHONE_EMAIL_WORKSPACE_MAX_BYTES": "1"})
	w.receivedAt(t, "09:00", filesMail)
	r := newReader(t, w, emailRead)
	thread := soleThread(t, r)

	quote := openMail(t, r, thread.ID, thread.Latest.ID)

	if quote.RawState != "over_limit" || quote.OriginalPath != nil {
		t.Errorf("the quote keeps its original as %q at %v, want it over the limit with no path",
			quote.RawState, quote.OriginalPath)
	}
	for _, file := range quote.Attachments {
		if file.State != "over_limit" || file.DownloadPath != nil {
			t.Errorf("the file %+v, want it over the limit with no download", file)
		}
	}
}

func TestOneRequestReadsNoMoreBodiesThanTheBound(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, map[string]string{"ALPHONE_EMAIL_BODIES_PER_REQUEST": "1"})
	w.receivedAt(t, "09:00", plainMail(mariaPerez, "Lamp quote", "lamp-quote@example.com", "", "Hello."))
	w.receivedAt(t, "09:05", plainMail(adaLovelace, "Delivery date", "delivery-date@example.com", "", "Soon?"))
	r := newReader(t, w, emailRead)

	bodies := r.ask(t, `{ emailThreads { items { latest { body { text } } } } }`, nil)
	var one servedPage[servedThread]
	r.read(t, `{ emailThreads(limit: 1) { items { latest { body { text } } } } }`, nil, "emailThreads", &one)

	if !slices.Contains(bodies.reasons(), "bodies_per_request_exceeded") {
		t.Errorf("two bodies in one request answered %+v, want bodies_per_request_exceeded", bodies.Errors)
	}
	for _, fault := range bodies.Errors {
		if meta, _ := fault.Extensions["meta"].(map[string]any); meta["max"] != float64(1) {
			t.Errorf("the rejection %+v, want it to name the bound of 1", fault)
		}
	}
	if len(one.Items) != 1 || one.Items[0].Latest.Body == nil || one.Items[0].Latest.Body.Text == "" {
		t.Errorf("one body in one request = %+v, want its text", one.Items)
	}
}

func TestOneRequestReadsNoMorePeopleAndFileListsThanTheBound(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, map[string]string{"ALPHONE_EMAIL_LISTS_PER_REQUEST": "1"})
	w.receivedAt(t, "09:00", plainMail(mariaPerez, "Lamp quote", "lamp-quote@example.com", "", "Hello."))
	w.receivedAt(t, "09:05", plainMail(adaLovelace, "Delivery date", "delivery-date@example.com", "", "Soon?"))
	r := newReader(t, w, emailRead)

	lists := r.ask(t, `{ emailThreads { items { latest { participants { email } } } } }`, nil)
	var one servedPage[servedThread]
	r.read(t, `{ emailThreads(limit: 1) { items { latest { participants { email } } } } }`, nil, "emailThreads", &one)

	if !slices.Contains(lists.reasons(), "lists_per_request_exceeded") {
		t.Errorf("two people lists in one request answered %+v, want lists_per_request_exceeded", lists.Errors)
	}
	for _, fault := range lists.Errors {
		if meta, _ := fault.Extensions["meta"].(map[string]any); meta["max"] != float64(1) {
			t.Errorf("the rejection %+v, want it to name the bound of 1", fault)
		}
	}
	if len(one.Items) != 1 || len(one.Items[0].Latest.Participants) == 0 {
		t.Errorf("one people list in one request = %+v, want its people", one.Items)
	}
}

// repeatedTextQuery reads the text of one mail of a thread under two names.
const repeatedTextQuery = `query Twice($thread: UUID!, $id: UUID!) {
	emailThread(id: $thread) { message(id: $id) { body { first: text second: text } } }
}`

// repeatedTextFragmentsQuery reads the text of one mail of a thread under two names through two fragments.
const repeatedTextFragmentsQuery = `query Twice($thread: UUID!, $id: UUID!) {
	emailThread(id: $thread) { message(id: $id) { body { ...FirstText ...SecondText } } }
}
fragment FirstText on EmailBody { first: text }
fragment SecondText on EmailBody { second: text }`

// repeatedNameQuery reads the name of each person one mail of a thread names under two names.
const repeatedNameQuery = `query Twice($thread: UUID!, $id: UUID!) {
	emailThread(id: $thread) { message(id: $id) { participants { first: name second: name } } }
}`

// twoBodiesQuery reads the text of two mails of a thread, each under its own name.
const twoBodiesQuery = `query Both($thread: UUID!, $newest: UUID!, $oldest: UUID!) {
	emailThread(id: $thread) {
		newest: message(id: $newest) { body { text } }
		oldest: message(id: $oldest) { body { text } }
	}
}`

func TestAFieldAskedUnderTwoNamesIsRejectedWhileTwoMailsAnswerTheirBodies(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	w.receivedAt(t, "09:00", plainMail(mariaPerez, "Lamp quote", "lamp-quote@example.com", "", "Hello."))
	w.receivedAt(t, "09:05", plainMail(mariaPerez, "Re: Lamp quote", "lamp-quote-reply@example.com",
		"lamp-quote@example.com", "Any news?"))
	r := newReader(t, w, emailRead)
	thread := soleThread(t, r)
	var listed *servedThread
	r.read(t, threadQuery, map[string]any{"id": thread.ID}, "emailThread", &listed)
	newest, oldest := listed.Messages[0].ID, listed.Messages[1].ID

	for name, test := range map[string]struct {
		query string
		field string
	}{
		"the text under two names":                {repeatedTextQuery, "EmailBody.text"},
		"the text through two fragments":          {repeatedTextFragmentsQuery, "EmailBody.text"},
		"the name of each person under two names": {repeatedNameQuery, "EmailParticipant.name"},
	} {
		answer := r.ask(t, test.query, map[string]any{"thread": thread.ID, "id": newest})

		if !slices.Equal(answer.reasons(), []any{"repeated_field"}) {
			t.Errorf("%s answered %+v, want it rejected as repeated_field", name, answer.Errors)
			continue
		}
		if meta, _ := answer.Errors[0].Extensions["meta"].(map[string]any); meta["field"] != test.field {
			t.Errorf("%s was rejected naming %v, want %s", name, answer.Errors[0].Extensions["meta"], test.field)
		}
	}
	var both struct {
		Newest servedMail `json:"newest"`
		Oldest servedMail `json:"oldest"`
	}
	r.read(t, twoBodiesQuery, map[string]any{"thread": thread.ID, "newest": newest, "oldest": oldest},
		"emailThread", &both)
	if both.Newest.Body == nil || strings.TrimSpace(both.Newest.Body.Text) != "Any news?" ||
		both.Oldest.Body == nil || strings.TrimSpace(both.Oldest.Body.Text) != "Hello." {
		t.Errorf("two mails under their own names = %+v, want the reply and the quote read", both)
	}
}

func TestTheMailFieldsOfLoadersNeedARequestScope(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	w.receivedAt(t, "09:00", filesMail)
	r := newReader(t, w, emailRead)
	thread := soleThread(t, r)
	r.unscoped = true

	answer := r.ask(t, mailQuery, map[string]any{"thread": thread.ID, "id": thread.Latest.ID})

	var failed []string
	for _, fault := range answer.Errors {
		failed = append(failed, fmt.Sprint(fault.Path[len(fault.Path)-1]))
	}
	slices.Sort(failed)
	if want := []string{"attachments", "body", "contactIds", "participants"}; !slices.Equal(failed, want) {
		t.Errorf("a mail read with no request scope failed at %q, want %q", failed, want)
	}
}

func TestTheRootFieldsAnswerTheFailureOfAClosedPool(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	w.receivedAt(t, "09:00", plainMail(mariaPerez, "Lamp quote", "lamp-quote@example.com", "", "Hello."))
	r := newReader(t, w, emailRead)
	thread, contact := soleThread(t, r).ID, w.contactID(t, "maria.perez@example.com")
	if err := w.plugin.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}

	for _, query := range []string{
		`{ emailMailbox { id } }`,
		`{ emailThreads { total } }`,
		fmt.Sprintf(`{ emailThread(id: %q) { id } }`, thread),
		fmt.Sprintf(`{ emailContactMessages(contactId: %q) { total } }`, contact),
	} {
		answer := r.ask(t, query, nil)

		if len(answer.Errors) != 1 || answer.Errors[0].Extensions["code"] != "INTERNAL" {
			t.Errorf("%s over a closed pool answered %+v, want one internal error", query, answer.Errors)
		}
	}
}

// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// emailReadFeature is the feature reading stored mail and downloading its files answers to.
const emailReadFeature = "../../test/features/features/email-read.feature"

// graphPath is where AlphOne answers the graph.
const graphPath = "/api/graphql"

// attachmentRoute is the path an attachment of a stored mail downloads from, before its id.
const attachmentRoute = "/api/plugins/email/attachments/"

// messageRoute is the path a stored mail is reached under, before its id.
const messageRoute = "/api/plugins/email/messages/"

// originalSuffix ends the path the original of a stored mail downloads from.
const originalSuffix = "/original"

// secretLabel starts the line token:create prints a new secret on.
const secretLabel = "secret: "

// scopeRequired starts the error the graph answers a token that lacks the scope a field needs.
const scopeRequired = "scope required: "

// answerPreview bounds how much of an answer a failing step quotes.
const answerPreview = 200

// placeInWorkspace stands the account at an address in one workspace.
const placeInWorkspace = `INSERT INTO core.tenant_members (user_id, tenant_id)
	SELECT id, $2 FROM auth.users WHERE email = $1`

// mailboxQuery reads the mailbox of the caller's workspace.
const mailboxQuery = `query { emailMailbox { id address aliases } }`

// inboxQuery reads one page of the inbox with the newest mail of each thread.
const inboxQuery = `query Inbox($limit: Int, $offset: Int) {
	emailThreads(limit: $limit, offset: $offset) {
		total limit
		items { id subject messageCount hasAttachments latest { id subject from { name email } snippet } }
	}
}`

// mailFinderQuery reads the ids and subjects of the mail of the newest threads.
const mailFinderQuery = `query { emailThreads(limit: 10) { items { id subject messages(limit: 10) { id subject } } } }`

// threadQuery reads one thread with one page of its mail.
const threadQuery = `query Thread($id: UUID!, $limit: Int, $offset: Int) {
	emailThread(id: $id) { id subject messageCount messages(limit: $limit, offset: $offset) { id subject } }
}`

// mailQuery reads one mail of a thread with its body and its files.
const mailQuery = `query Mail($thread: UUID!, $id: UUID!) {
	emailThread(id: $thread) {
		message(id: $id) {
			id subject snippet bodyState skippedParts rawState originalPath
			body { text html }
			attachments { id filename state inline downloadPath }
		}
	}
}`

// mailFilesQuery reads the files of one mail of a thread.
const mailFilesQuery = `query Files($thread: UUID!, $id: UUID!) {
	emailThread(id: $thread) { message(id: $id) { id attachments { id filename state inline downloadPath } } }
}`

// mailPeopleQuery reads the people one mail of a thread names.
const mailPeopleQuery = `query People($thread: UUID!, $id: UUID!) {
	emailThread(id: $thread) { message(id: $id) { id subject participants { role name email } } }
}`

// newestBodiesQuery reads the text of the newest mail of every thread of the inbox in one request.
const newestBodiesQuery = `query { emailThreads { items { latest { body { text } } } } }`

// newestPeopleQuery reads the people of the newest mail of every thread of the inbox in one request.
const newestPeopleQuery = `query { emailThreads { items { latest { participants { email } } } } }`

// contactQuery reads the contacts whose name matches one name.
const contactQuery = `query Contact($name: String) { contacts(q: $name, first: 10) { edges { node { id name } } } }`

// contactMailQuery reads the mail linked to one contact, newest first.
const contactMailQuery = `query ContactMail($contact: UUID!) {
	emailContactMessages(contactId: $contact) { total limit items { id threadId subject } }
}`

// graphReply is one answer of the graph, its data kept raw by root field.
type graphReply struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []graphFault               `json:"errors"`
}

// graphFault is one error the graph answers.
type graphFault struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions"`
}

// servedMailbox is a mailbox as the graph answers it.
type servedMailbox struct {
	ID      string   `json:"id"`
	Address string   `json:"address"`
	Aliases []string `json:"aliases"`
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
	MessageCount   int          `json:"messageCount"`
	HasAttachments bool         `json:"hasAttachments"`
	Latest         servedMail   `json:"latest"`
	Messages       []servedMail `json:"messages"`
	Message        *servedMail  `json:"message"`
}

// servedMail is a mail as the graph answers it.
type servedMail struct {
	ID           string              `json:"id"`
	ThreadID     string              `json:"threadId"`
	Subject      string              `json:"subject"`
	From         *servedAddress      `json:"from"`
	Snippet      string              `json:"snippet"`
	BodyState    string              `json:"bodyState"`
	SkippedParts int                 `json:"skippedParts"`
	RawState     string              `json:"rawState"`
	OriginalPath *string             `json:"originalPath"`
	Participants []servedParticipant `json:"participants"`
	Body         *servedBody         `json:"body"`
	Attachments  []servedFile        `json:"attachments"`
}

// servedAddress is a person a mail names, as the graph answers it.
type servedAddress struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// servedParticipant is a person a mail names beside the field that names them, as the graph answers it.
type servedParticipant struct {
	Role  string `json:"role"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// servedBody is the text and the safe HTML of a mail, as the graph answers them.
type servedBody struct {
	Text string `json:"text"`
	HTML string `json:"html"`
}

// servedFile is an attachment of a mail, as the graph answers it.
type servedFile struct {
	ID           string  `json:"id"`
	Filename     string  `json:"filename"`
	State        string  `json:"state"`
	Inline       bool    `json:"inline"`
	DownloadPath *string `json:"downloadPath"`
}

// httpAnswer is what AlphOne answered one request.
type httpAnswer struct {
	status int
	header http.Header
	body   []byte
}

// readScenario is what one email read scenario keeps between its steps, beside what the email steps keep.
type readScenario struct {
	*emailScenario
	tokens       map[string]string
	minted       int
	reply        graphReply
	mailIDs      map[string]string
	sharedThread string
	sharedFile   string
	download     httpAnswer
}

// initializeEmailRead returns the binding of the email read steps beside the email steps, t holding their databases.
func initializeEmailRead(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := newEmailScenario(t)
		bindEmailSteps(sc, s)
		r := &readScenario{emailScenario: s, tokens: map[string]string{}, mailIDs: map[string]string{}}
		r.bindTokens(sc)
		r.bindReads(sc)
		r.bindReadAnswers(sc)
		r.bindMailAnswers(sc)
		r.bindDownloads(sc)
		r.bindDownloadAnswers(sc)
	}
}

// bindTokens binds the steps that place accounts in workspaces and mint their tokens.
func (r *readScenario) bindTokens(sc *godog.ScenarioContext) {
	sc.Given(`^the account "([^"]*)" is placed in the workspace "([^"]*)"$`, r.placeAccount)
	sc.Given(`^the member "([^"]*)" holds a token scoped to "([^"]*)"$`, r.holdToken)
	sc.When(`^the operator mints a token for "([^"]*)" scoped to "([^"]*)"$`, r.mintToken)
	sc.Then(`^the token list of "([^"]*)" shows a token scoped to "([^"]*)"$`, r.listsTokenScopedTo)
}

// bindReads binds the steps that read the mailbox, the inbox, a thread, a mail and the mail of a contact.
func (r *readScenario) bindReads(sc *godog.ScenarioContext) {
	sc.When(`^the member "([^"]*)" reads the mailbox$`, r.readMailbox)
	sc.When(`^the member "([^"]*)" reads the inbox$`, r.readInbox)
	sc.When(`^the member "([^"]*)" reads (\d+) threads of the inbox after skipping (\d+)$`, r.readInboxPage)
	sc.When(`^the member "([^"]*)" opens the thread "([^"]*)"$`, r.openThread)
	sc.When(`^the member "([^"]*)" reads (\d+) mails of the thread "([^"]*)" after skipping (\d+)$`, r.readThreadPage)
	sc.Given(`^the member "([^"]*)" shares the thread "([^"]*)"$`, r.shareThread)
	sc.When(`^the member "([^"]*)" opens the shared thread$`, r.openSharedThread)
	sc.When(`^the member "([^"]*)" reads the mail "([^"]*)"$`, r.readMail)
	sc.When(`^the member "([^"]*)" reads the text of the newest mail of every inbox thread in one request$`,
		r.readNewestBodies)
	sc.When(`^the member "([^"]*)" reads the people of the newest mail of every inbox thread in one request$`,
		r.readNewestPeople)
	sc.When(`^the member "([^"]*)" reads the people of the mail "([^"]*)"$`, r.readMailPeople)
	sc.When(`^the member "([^"]*)" reads the mail of the contact "([^"]*)"$`, r.readContactMail)
}

// bindReadAnswers binds the steps that check what a read of the mailbox, the inbox, a thread or a contact answered.
func (r *readScenario) bindReadAnswers(sc *godog.ScenarioContext) {
	sc.Then(`^the read is rejected for want of the scope "([^"]*)"$`, r.rejectedForScope)
	sc.Then(`^the read is rejected with the reason "([^"]*)"$`, r.rejectedWithReason)
	sc.Then(`^the workspace answers no mailbox$`, r.answersNoMailbox)
	sc.Then(`^the mailbox answers the address "([^"]*)" with the alias "([^"]*)"$`, r.answersMailbox)
	sc.Then(`^the inbox lists the threads "([^"]*)" in that order$`, r.inboxLists)
	sc.Then(`^the inbox answers a total of (\d+) threads? on pages of (\d+)$`, r.inboxAnswersTotal)
	sc.Then(`^the inbox thread "([^"]*)" holds (\d+) mails? and (files|no files)$`, r.inboxThreadHolds)
	sc.Then(`^the newest mail of the inbox thread "([^"]*)" is "([^"]*)" from "([^"]*)"$`, r.newestMailIs)
	sc.Then(`^the thread lists the mails "([^"]*)" in that order$`, r.threadListsMails)
	sc.Then(`^no thread is found$`, r.findsNoThread)
	sc.Then(`^the contact's mail lists "([^"]*)" with a total of (\d+)$`, r.contactMailLists)
}

// bindMailAnswers binds the steps that check what a read of one mail answered.
func (r *readScenario) bindMailAnswers(sc *godog.ScenarioContext) {
	sc.Then(`^the mail reads the text "([^"]*)"$`, r.mailReadsText)
	sc.Then(`^the mail names "([^"]*)" at "([^"]*)" as its sender$`, r.mailNamesSender)
	sc.Then(`^the HTML of the mail keeps the remote image "([^"]*)"$`, r.htmlKeepsImage)
	sc.Then(`^the HTML of the mail holds no script$`, r.htmlHoldsNoScript)
	sc.Then(`^the mail answers its body as "([^"]*)"$`, r.mailBodyIs)
	sc.Then(`^the mail answers an empty preview$`, r.mailPreviewIsEmpty)
	sc.Then(`^the mail answers (\d+) parts? left out$`, r.mailLeavesOutParts)
	sc.Then(`^the mail offers its original$`, r.mailOffersOriginal)
	sc.Then(`^the mail offers the file "([^"]*)" for download$`, r.mailOffersFile)
	sc.Then(`^the mail offers no download of the file "([^"]*)" as it is (too large|over the workspace limit)$`,
		r.mailOffersNoFile)
}

// bindDownloads binds the steps that download the files and the original of stored mail.
func (r *readScenario) bindDownloads(sc *godog.ScenarioContext) {
	sc.When(`^the member "([^"]*)" downloads the file "([^"]*)" of the mail "([^"]*)"$`, r.downloadFile)
	sc.When(`^the member "([^"]*)" downloads the only file of the mail "([^"]*)"$`, r.downloadOnlyFile)
	sc.When(`^the member "([^"]*)" downloads the original of the mail "([^"]*)"$`, r.downloadOriginal)
	sc.Given(`^the member "([^"]*)" shares the file "([^"]*)" of the mail "([^"]*)"$`, r.shareFile)
	sc.When(`^the member "([^"]*)" downloads the shared file$`, r.downloadShared)
}

// bindDownloadAnswers binds the steps that check what a download answered.
func (r *readScenario) bindDownloadAnswers(sc *godog.ScenarioContext) {
	sc.Then(`^the download answers the type "([^"]*)"$`, r.downloadAnswersType)
	sc.Then(`^the download is saved as an attachment named "([^"]*)"$`, r.savedAs)
	sc.Then(`^the download is saved as an original named after the id of the mail "([^"]*)"$`, r.savedAsOriginalOf)
	sc.Then(`^the download forbids sniffing, runs in a sandbox and is never stored$`, r.downloadIsGuarded)
	sc.Then(`^the download is shown inline$`, r.downloadShownInline)
	sc.Then(`^the download starts with "([^"]*)"$`, r.downloadStartsWith)
	sc.Then(`^the download holds the mail as it reached AlphOne$`, r.downloadHoldsFedMail)
	sc.Then(`^the download answers not found$`, r.downloadNotFound)
	sc.Then(`^the download is forbidden$`, r.downloadForbidden)
}

// placeAccount stands the account at email in the workspace called name.
func (r *readScenario) placeAccount(ctx context.Context, email, name string) error {
	placed, err := r.pool.Exec(ctx, placeInWorkspace, email, r.workspaces[name])
	if err != nil || placed.RowsAffected() != 1 {
		return fmt.Errorf("placing %s in %s placed %d accounts (%v), want 1", email, name, placed.RowsAffected(), err)
	}
	return nil
}

// mintToken mints a token for the account at email holding the scopes the text spaces apart, keeping its secret.
func (r *readScenario) mintToken(email, scopes string) {
	r.minted++
	args := []string{"token:create", "-email", email, "-name", fmt.Sprintf("reader-%d", r.minted)}
	for _, scope := range strings.Fields(scopes) {
		args = append(args, "-scope", scope)
	}
	r.result = r.answer("", args...)
	if secret, found := mintedSecret(r.result.Stdout); found {
		r.tokens[email] = secret
	}
}

// holdToken mints a token for the account at email holding scopes and fails unless token:create exits 0.
func (r *readScenario) holdToken(email, scopes string) error {
	r.mintToken(email, scopes)
	return r.wantExitZero()
}

// mintedSecret answers the secret token:create printed on stdout, false when it printed none.
func mintedSecret(stdout string) (string, bool) {
	for line := range strings.Lines(stdout) {
		if secret, found := strings.CutPrefix(strings.TrimSpace(line), secretLabel); found {
			return secret, true
		}
	}
	return "", false
}

// listsTokenScopedTo fails unless the account at email holds a token scoped to exactly the scopes spaced in the text.
func (r *readScenario) listsTokenScopedTo(email, scopes string) error {
	listed := r.answer("", "token:list", "-email", email, "-json")
	var document struct {
		Tokens []struct {
			Scopes []string `json:"scopes"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(listed.Stdout), &document); err != nil {
		return fmt.Errorf("token:list exited with %d, stdout %q, stderr %q: %w",
			listed.Code, listed.Stdout, listed.Stderr, err)
	}
	want := strings.Fields(scopes)
	for _, held := range document.Tokens {
		if slices.Equal(held.Scopes, want) {
			return nil
		}
	}
	return fmt.Errorf("the tokens of %s hold the scopes %+v, want one scoped to %v", email, document.Tokens, want)
}

// servingAddress answers the address AlphOne serves the scenario at, starting it on first use.
func (r *readScenario) servingAddress() (string, error) {
	if r.stopServing == nil {
		if err := r.serveAlphOne(); err != nil {
			return "", err
		}
	}
	return r.env["ALPHONE_ADDR"], nil
}

// call answers what AlphOne answers a request of method to path carrying body, sent with the token of email.
func (r *readScenario) call(ctx context.Context, email, method, path string, body []byte) (httpAnswer, error) {
	secret, held := r.tokens[email]
	if !held {
		return httpAnswer{}, fmt.Errorf("the member %s holds no token", email)
	}
	address, err := r.servingAddress()
	if err != nil {
		return httpAnswer{}, err
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://"+address+path, bytes.NewReader(body))
	if err != nil {
		return httpAnswer{}, err
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return httpAnswer{}, err
	}
	defer func() { _ = response.Body.Close() }()
	read, err := io.ReadAll(response.Body)
	return httpAnswer{status: response.StatusCode, header: response.Header, body: read}, err
}

// ask answers what the graph replies to query with variables, sent with the token of email.
func (r *readScenario) ask(ctx context.Context, email, query string, variables map[string]any) (graphReply, error) {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return graphReply{}, err
	}
	answered, err := r.call(ctx, email, http.MethodPost, graphPath, body)
	if err != nil {
		return graphReply{}, err
	}
	var reply graphReply
	if err := json.Unmarshal(answered.body, &reply); err != nil {
		return graphReply{}, fmt.Errorf("the graph answered %d with %q: %w", answered.status, quoted(answered.body), err)
	}
	return reply, nil
}

// read keeps what the graph replies to query with variables, sent with the token of email.
func (r *readScenario) read(ctx context.Context, email, query string, variables map[string]any) error {
	reply, err := r.ask(ctx, email, query, variables)
	r.reply = reply
	return err
}

// learn decodes into dest the root field called name of the reply to query, an error for any graph error.
func (r *readScenario) learn(
	ctx context.Context, email, query string, variables map[string]any, name string, dest any,
) error {
	reply, err := r.ask(ctx, email, query, variables)
	if err != nil {
		return err
	}
	return reply.decode(name, dest)
}

// decode reads the root field called name of the reply into dest, an error naming the reply's errors when it holds any.
func (g graphReply) decode(name string, dest any) error {
	if len(g.Errors) > 0 {
		return fmt.Errorf("the graph answered the errors %s", g.faults())
	}
	if err := json.Unmarshal(g.Data[name], dest); err != nil {
		return fmt.Errorf("reading %s from the graph: %w", name, err)
	}
	return nil
}

// faults answers the messages and the reasons of the reply's errors on one line.
func (g graphReply) faults() string {
	described := make([]string, len(g.Errors))
	for i, fault := range g.Errors {
		described[i] = fmt.Sprintf("%q (%v)", fault.Message, fault.Extensions["reason"])
	}
	return strings.Join(described, ", ")
}

// rejects reports whether one error of the reply passes match.
func (g graphReply) rejects(match func(graphFault) bool) bool {
	return slices.ContainsFunc(g.Errors, match)
}

// replied answers the errors of the last reply, or its data when it holds none, for a failing step to quote.
func (r *readScenario) replied() string {
	if len(r.reply.Errors) > 0 {
		return "the errors " + r.reply.faults()
	}
	data, _ := json.Marshal(r.reply.Data)
	return "the data " + string(data)
}

// findThread answers the id of the thread titled subject that the inbox of email lists.
func (r *readScenario) findThread(ctx context.Context, email, subject string) (string, error) {
	var page servedPage[servedThread]
	if err := r.learn(ctx, email, inboxQuery, nil, "emailThreads", &page); err != nil {
		return "", fmt.Errorf("finding the thread %q: %w", subject, err)
	}
	at := slices.IndexFunc(page.Items, func(thread servedThread) bool { return thread.Subject == subject })
	if at < 0 {
		return "", fmt.Errorf("the inbox lists the threads %q, want %q among them", listedThreads(page.Items), subject)
	}
	return page.Items[at].ID, nil
}

// findMail answers the thread and the id of the one mail titled subject that the inbox of email holds.
func (r *readScenario) findMail(ctx context.Context, email, subject string) (string, string, error) {
	var page servedPage[servedThread]
	if err := r.learn(ctx, email, mailFinderQuery, nil, "emailThreads", &page); err != nil {
		return "", "", fmt.Errorf("finding the mail %q: %w", subject, err)
	}
	var thread, id string
	held := 0
	for _, listed := range page.Items {
		for _, mail := range listed.Messages {
			if mail.Subject == subject {
				thread, id, held = listed.ID, mail.ID, held+1
			}
		}
	}
	if held != 1 {
		return "", "", fmt.Errorf("the inbox holds %d mails titled %q, want 1", held, subject)
	}
	r.mailIDs[subject] = id
	return thread, id, nil
}

// filesOfMail answers the files of the mail titled subject as email reads them.
func (r *readScenario) filesOfMail(ctx context.Context, email, subject string) ([]servedFile, error) {
	thread, id, err := r.findMail(ctx, email, subject)
	if err != nil {
		return nil, err
	}
	var found *servedThread
	variables := map[string]any{"thread": thread, "id": id}
	if err := r.learn(ctx, email, mailFilesQuery, variables, "emailThread", &found); err != nil {
		return nil, fmt.Errorf("reading the files of %q: %w", subject, err)
	}
	if found == nil || found.Message == nil {
		return nil, fmt.Errorf("the thread of %q answered no mail %s", subject, id)
	}
	return found.Message.Attachments, nil
}

// findFile answers the file called filename of the mail titled subject as email reads it.
func (r *readScenario) findFile(ctx context.Context, email, filename, subject string) (servedFile, error) {
	files, err := r.filesOfMail(ctx, email, subject)
	if err != nil {
		return servedFile{}, err
	}
	file, err := fileCalled(files, filename)
	if err != nil {
		return servedFile{}, fmt.Errorf("the mail %q %w", subject, err)
	}
	return file, nil
}

// findContact answers the id of the contact called name that email reads among the contacts of the workspace.
func (r *readScenario) findContact(ctx context.Context, email, name string) (string, error) {
	var found struct {
		Edges []struct {
			Node struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"node"`
		} `json:"edges"`
	}
	if err := r.learn(ctx, email, contactQuery, map[string]any{"name": name}, "contacts", &found); err != nil {
		return "", fmt.Errorf("finding the contact %s: %w", name, err)
	}
	for _, edge := range found.Edges {
		if edge.Node.Name == name {
			return edge.Node.ID, nil
		}
	}
	return "", fmt.Errorf("the contacts %+v hold no %s", found.Edges, name)
}

// readMailbox reads the mailbox of the workspace of email.
func (r *readScenario) readMailbox(ctx context.Context, email string) error {
	return r.read(ctx, email, mailboxQuery, nil)
}

// readInbox reads the first page of the inbox of email.
func (r *readScenario) readInbox(ctx context.Context, email string) error {
	return r.read(ctx, email, inboxQuery, nil)
}

// readInboxPage reads limit threads of the inbox of email after skipping offset.
func (r *readScenario) readInboxPage(ctx context.Context, email string, limit, offset int) error {
	return r.read(ctx, email, inboxQuery, map[string]any{"limit": limit, "offset": offset})
}

// openThread reads the thread titled subject in the inbox of email with its mail.
func (r *readScenario) openThread(ctx context.Context, email, subject string) error {
	id, err := r.findThread(ctx, email, subject)
	if err != nil {
		return err
	}
	return r.read(ctx, email, threadQuery, map[string]any{"id": id})
}

// readThreadPage reads limit mails of the thread titled subject in the inbox of email after skipping offset.
func (r *readScenario) readThreadPage(ctx context.Context, email string, limit int, subject string, offset int) error {
	id, err := r.findThread(ctx, email, subject)
	if err != nil {
		return err
	}
	return r.read(ctx, email, threadQuery, map[string]any{"id": id, "limit": limit, "offset": offset})
}

// shareThread keeps the id of the thread titled subject in the inbox of email for another member to open.
func (r *readScenario) shareThread(ctx context.Context, email, subject string) error {
	id, err := r.findThread(ctx, email, subject)
	r.sharedThread = id
	return err
}

// openSharedThread reads the shared thread as email.
func (r *readScenario) openSharedThread(ctx context.Context, email string) error {
	return r.read(ctx, email, threadQuery, map[string]any{"id": r.sharedThread})
}

// readMail reads the mail titled subject as email, with its body and its files.
func (r *readScenario) readMail(ctx context.Context, email, subject string) error {
	return r.readMailThrough(ctx, email, subject, mailQuery)
}

// readMailPeople reads the people the mail titled subject names, as email.
func (r *readScenario) readMailPeople(ctx context.Context, email, subject string) error {
	return r.readMailThrough(ctx, email, subject, mailPeopleQuery)
}

// readMailThrough reads the mail titled subject as email through query.
func (r *readScenario) readMailThrough(ctx context.Context, email, subject, query string) error {
	thread, id, err := r.findMail(ctx, email, subject)
	if err != nil {
		return err
	}
	return r.read(ctx, email, query, map[string]any{"thread": thread, "id": id})
}

// readNewestBodies reads the text of the newest mail of every inbox thread of email in one request.
func (r *readScenario) readNewestBodies(ctx context.Context, email string) error {
	return r.read(ctx, email, newestBodiesQuery, nil)
}

// readNewestPeople reads the people of the newest mail of every inbox thread of email in one request.
func (r *readScenario) readNewestPeople(ctx context.Context, email string) error {
	return r.read(ctx, email, newestPeopleQuery, nil)
}

// readContactMail reads the mail linked to the contact called name as email.
func (r *readScenario) readContactMail(ctx context.Context, email, name string) error {
	id, err := r.findContact(ctx, email, name)
	if err != nil {
		return err
	}
	return r.read(ctx, email, contactMailQuery, map[string]any{"contact": id})
}

// rejectedForScope fails unless the last read was rejected for want of scope.
func (r *readScenario) rejectedForScope(scope string) error {
	if !r.reply.rejects(func(fault graphFault) bool { return fault.Message == scopeRequired+scope }) {
		return fmt.Errorf("the read answered %s, want it rejected for want of %s", r.replied(), scope)
	}
	return nil
}

// rejectedWithReason fails unless the last read was rejected for reason.
func (r *readScenario) rejectedWithReason(reason string) error {
	if !r.reply.rejects(func(fault graphFault) bool { return fault.Extensions["reason"] == reason }) {
		return fmt.Errorf("the read answered %s, want it rejected for %s", r.replied(), reason)
	}
	return nil
}

// answeredMailbox decodes the mailbox the last read answered, nil when it answered none.
func (r *readScenario) answeredMailbox() (*servedMailbox, error) {
	var box *servedMailbox
	err := r.reply.decode("emailMailbox", &box)
	return box, err
}

// answersNoMailbox fails unless the last read answered no mailbox.
func (r *readScenario) answersNoMailbox() error {
	box, err := r.answeredMailbox()
	if err != nil {
		return err
	}
	if box != nil {
		return fmt.Errorf("the workspace answers the mailbox %+v, want none", *box)
	}
	return nil
}

// answersMailbox fails unless the last read answered the mailbox at address with alias as its one alias.
func (r *readScenario) answersMailbox(address, alias string) error {
	box, err := r.answeredMailbox()
	if err != nil {
		return err
	}
	if box == nil || box.ID == "" || box.Address != address || !slices.Equal(box.Aliases, []string{alias}) {
		return fmt.Errorf("the workspace answers the mailbox %+v, want %s with the alias %s", box, address, alias)
	}
	return nil
}

// inboxPage decodes the inbox page the last read answered.
func (r *readScenario) inboxPage() (servedPage[servedThread], error) {
	var page servedPage[servedThread]
	err := r.reply.decode("emailThreads", &page)
	return page, err
}

// inboxLists fails unless the last inbox read lists the threads titled as the comma list want, in that order.
func (r *readScenario) inboxLists(want string) error {
	page, err := r.inboxPage()
	if err != nil {
		return err
	}
	if listed := listedThreads(page.Items); listed != want {
		return fmt.Errorf("the inbox lists the threads %q, want %q", listed, want)
	}
	return nil
}

// inboxAnswersTotal fails unless the last inbox read counts total threads on pages of limit.
func (r *readScenario) inboxAnswersTotal(total, limit int) error {
	page, err := r.inboxPage()
	if err != nil {
		return err
	}
	if page.Total != total || page.Limit != limit {
		return fmt.Errorf("the inbox answers a total of %d on pages of %d, want %d on pages of %d",
			page.Total, page.Limit, total, limit)
	}
	return nil
}

// inboxThread answers the thread titled subject that the last inbox read lists.
func (r *readScenario) inboxThread(subject string) (servedThread, error) {
	page, err := r.inboxPage()
	if err != nil {
		return servedThread{}, err
	}
	at := slices.IndexFunc(page.Items, func(thread servedThread) bool { return thread.Subject == subject })
	if at < 0 {
		return servedThread{}, fmt.Errorf("the inbox lists the threads %q, want %q among them",
			listedThreads(page.Items), subject)
	}
	return page.Items[at], nil
}

// inboxThreadHolds fails unless the inbox thread titled subject counts want mails and holds files as kind says.
func (r *readScenario) inboxThreadHolds(subject string, want int, kind string) error {
	thread, err := r.inboxThread(subject)
	if err != nil {
		return err
	}
	if thread.MessageCount != want || thread.HasAttachments != (kind == "files") {
		return fmt.Errorf("the thread %q holds %d mails and files %v, want %d mails and %s",
			subject, thread.MessageCount, thread.HasAttachments, want, kind)
	}
	return nil
}

// newestMailIs fails unless the newest mail of the inbox thread titled subject is titled newest and sent from address.
func (r *readScenario) newestMailIs(subject, newest, address string) error {
	thread, err := r.inboxThread(subject)
	if err != nil {
		return err
	}
	latest := thread.Latest
	if latest.Subject != newest || latest.From == nil || latest.From.Email != address {
		return fmt.Errorf("the newest mail of %q is %q from %+v, want %q from %s",
			subject, latest.Subject, latest.From, newest, address)
	}
	return nil
}

// openedThread decodes the thread the last read answered, nil when it found none.
func (r *readScenario) openedThread() (*servedThread, error) {
	var thread *servedThread
	err := r.reply.decode("emailThread", &thread)
	return thread, err
}

// threadListsMails fails unless the last thread read lists the mails titled as the comma list want, in that order.
func (r *readScenario) threadListsMails(want string) error {
	thread, err := r.openedThread()
	if err != nil {
		return err
	}
	if thread == nil {
		return fmt.Errorf("the read found no thread, want one listing %q", want)
	}
	if listed := listedMails(thread.Messages); listed != want {
		return fmt.Errorf("the thread lists the mails %q, want %q", listed, want)
	}
	return nil
}

// findsNoThread fails unless the last read answered no thread and no error.
func (r *readScenario) findsNoThread() error {
	thread, err := r.openedThread()
	if err != nil {
		return err
	}
	if thread != nil {
		return fmt.Errorf("the read found the thread %s titled %q, want none", thread.ID, thread.Subject)
	}
	return nil
}

// contactMailLists fails unless the last read of a contact's mail lists the mails titled as want and counts total.
func (r *readScenario) contactMailLists(want string, total int) error {
	var page servedPage[servedMail]
	if err := r.reply.decode("emailContactMessages", &page); err != nil {
		return err
	}
	if listed := listedMails(page.Items); listed != want || page.Total != total {
		return fmt.Errorf("the contact's mail lists %q with a total of %d, want %q with a total of %d",
			listed, page.Total, want, total)
	}
	return nil
}

// checkOpenedMail fails with what check finds wrong with the mail the last read answered.
func (r *readScenario) checkOpenedMail(check func(servedMail) error) error {
	thread, err := r.openedThread()
	if err != nil {
		return err
	}
	if thread == nil || thread.Message == nil {
		return fmt.Errorf("the read answered %s, want a mail", r.replied())
	}
	if err := check(*thread.Message); err != nil {
		return fmt.Errorf("the mail %q %w", thread.Message.Subject, err)
	}
	return nil
}

// mailReadsText fails unless the mail the last read answered reads text, blanks around it aside.
func (r *readScenario) mailReadsText(text string) error {
	return r.checkOpenedMail(func(mail servedMail) error {
		if mail.Body == nil || strings.TrimSpace(mail.Body.Text) != text {
			return fmt.Errorf("reads the body %+v, want the text %q", mail.Body, text)
		}
		return nil
	})
}

// mailNamesSender fails unless the mail the last read answered names name at address as its sender.
func (r *readScenario) mailNamesSender(name, address string) error {
	return r.checkOpenedMail(func(mail servedMail) error {
		sender := servedParticipant{Role: "from", Name: name, Email: address}
		if !slices.Contains(mail.Participants, sender) {
			return fmt.Errorf("names the people %+v, want %s at %s as its sender", mail.Participants, name, address)
		}
		return nil
	})
}

// htmlKeepsImage fails unless the HTML of the mail the last read answered shows the image at source.
func (r *readScenario) htmlKeepsImage(source string) error {
	return r.checkOpenedMail(func(mail servedMail) error {
		if mail.Body == nil || !strings.Contains(mail.Body.HTML, `src="`+source+`"`) {
			return fmt.Errorf("reads the body %+v, want its HTML to show the image %s", mail.Body, source)
		}
		return nil
	})
}

// htmlHoldsNoScript fails when the HTML of the mail the last read answered holds a script.
func (r *readScenario) htmlHoldsNoScript() error {
	return r.checkOpenedMail(func(mail servedMail) error {
		if mail.Body == nil || strings.Contains(strings.ToLower(mail.Body.HTML), "<script") ||
			strings.Contains(mail.Body.HTML, "alert(") {
			return fmt.Errorf("reads the body %+v, want HTML that holds no script", mail.Body)
		}
		return nil
	})
}

// mailBodyIs fails unless the mail the last read answered states its body as state.
func (r *readScenario) mailBodyIs(state string) error {
	return r.checkOpenedMail(func(mail servedMail) error {
		if mail.BodyState != state {
			return fmt.Errorf("states its body as %q, want %q", mail.BodyState, state)
		}
		return nil
	})
}

// mailPreviewIsEmpty fails unless the mail the last read answered holds an empty preview.
func (r *readScenario) mailPreviewIsEmpty() error {
	return r.checkOpenedMail(func(mail servedMail) error {
		if mail.Snippet != "" {
			return fmt.Errorf("previews %q, want an empty preview", mail.Snippet)
		}
		return nil
	})
}

// mailLeavesOutParts fails unless the mail the last read answered counts want parts it left out.
func (r *readScenario) mailLeavesOutParts(want int) error {
	return r.checkOpenedMail(func(mail servedMail) error {
		if mail.SkippedParts != want {
			return fmt.Errorf("leaves out %d parts, want %d", mail.SkippedParts, want)
		}
		return nil
	})
}

// mailOffersOriginal fails unless the mail the last read answered keeps its original at its download path.
func (r *readScenario) mailOffersOriginal() error {
	return r.checkOpenedMail(func(mail servedMail) error {
		want := messageRoute + mail.ID + originalSuffix
		if mail.RawState != "stored" || mail.OriginalPath == nil || *mail.OriginalPath != want {
			return fmt.Errorf("keeps its original as %q at %s, want it stored at %s",
				mail.RawState, shownPath(mail.OriginalPath), want)
		}
		return nil
	})
}

// mailOffersFile fails unless the mail the last read answered keeps the file called filename at its download path.
func (r *readScenario) mailOffersFile(filename string) error {
	return r.checkOpenedMail(func(mail servedMail) error {
		file, err := fileCalled(mail.Attachments, filename)
		if err != nil {
			return err
		}
		want := attachmentRoute + file.ID
		if file.State != "stored" || file.DownloadPath == nil || *file.DownloadPath != want {
			return fmt.Errorf("keeps the file %s as %q at %s, want it stored at %s",
				filename, file.State, shownPath(file.DownloadPath), want)
		}
		return nil
	})
}

// mailOffersNoFile fails unless the mail the last read answered names the file called filename with no download.
func (r *readScenario) mailOffersNoFile(filename, reason string) error {
	return r.checkOpenedMail(func(mail servedMail) error {
		file, err := fileCalled(mail.Attachments, filename)
		if err != nil {
			return err
		}
		if file.State != skippedStates[reason] || file.DownloadPath != nil {
			return fmt.Errorf("keeps the file %s as %q at %s, want it %s with no download",
				filename, file.State, shownPath(file.DownloadPath), skippedStates[reason])
		}
		return nil
	})
}

// fileCalled answers the file called filename among files.
func fileCalled(files []servedFile, filename string) (servedFile, error) {
	at := slices.IndexFunc(files, func(file servedFile) bool { return file.Filename == filename })
	if at < 0 {
		return servedFile{}, fmt.Errorf("holds the files %+v, want %s among them", files, filename)
	}
	return files[at], nil
}

// downloadFile downloads the file called filename of the mail titled subject as email.
func (r *readScenario) downloadFile(ctx context.Context, email, filename, subject string) error {
	file, err := r.findFile(ctx, email, filename, subject)
	if err != nil {
		return err
	}
	return r.downloadFrom(ctx, email, attachmentRoute+file.ID)
}

// downloadOnlyFile downloads the one file of the mail titled subject as email.
func (r *readScenario) downloadOnlyFile(ctx context.Context, email, subject string) error {
	files, err := r.filesOfMail(ctx, email, subject)
	if err != nil {
		return err
	}
	if len(files) != 1 {
		return fmt.Errorf("the mail %q holds the files %+v, want one", subject, files)
	}
	return r.downloadFrom(ctx, email, attachmentRoute+files[0].ID)
}

// downloadOriginal downloads the original of the mail titled subject as email.
func (r *readScenario) downloadOriginal(ctx context.Context, email, subject string) error {
	_, id, err := r.findMail(ctx, email, subject)
	if err != nil {
		return err
	}
	return r.downloadFrom(ctx, email, messageRoute+id+originalSuffix)
}

// shareFile keeps the download path of the file called filename of the mail titled subject for another to use.
func (r *readScenario) shareFile(ctx context.Context, email, filename, subject string) error {
	file, err := r.findFile(ctx, email, filename, subject)
	r.sharedFile = attachmentRoute + file.ID
	return err
}

// downloadShared downloads the shared file as email.
func (r *readScenario) downloadShared(ctx context.Context, email string) error {
	return r.downloadFrom(ctx, email, r.sharedFile)
}

// downloadFrom downloads path as email and keeps what AlphOne answered.
func (r *readScenario) downloadFrom(ctx context.Context, email, path string) error {
	answered, err := r.call(ctx, email, http.MethodGet, path, nil)
	r.download = answered
	return err
}

// downloaded answers what the last download answered, for a failing step to quote.
func (r *readScenario) downloaded() string {
	return fmt.Sprintf("%d with the headers %v and the body %q",
		r.download.status, r.download.header, quoted(r.download.body))
}

// downloadAnswersType fails unless the last download succeeded carrying the media type.
func (r *readScenario) downloadAnswersType(mediaType string) error {
	if r.download.status != http.StatusOK || r.download.header.Get("Content-Type") != mediaType {
		return fmt.Errorf("the download answered %s, want 200 carrying %s", r.downloaded(), mediaType)
	}
	return nil
}

// savedAs fails unless the last download is saved as an attachment under filename.
func (r *readScenario) savedAs(filename string) error {
	disposition, params, err := mime.ParseMediaType(r.download.header.Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || params["filename"] != filename {
		return fmt.Errorf("the download answered %s, want an attachment named %q", r.downloaded(), filename)
	}
	return nil
}

// savedAsOriginalOf fails unless the last download is saved under the name of the original of the mail titled subject.
func (r *readScenario) savedAsOriginalOf(subject string) error {
	id, learned := r.mailIDs[subject]
	if !learned {
		return fmt.Errorf("no read learned the id of the mail %q", subject)
	}
	return r.savedAs("original-" + id + ".eml")
}

// downloadIsGuarded fails unless the last download forbids sniffing, runs in a sandbox and is never stored.
func (r *readScenario) downloadIsGuarded() error {
	header := r.download.header
	if header.Get("X-Content-Type-Options") != "nosniff" || header.Get("Content-Security-Policy") != "sandbox" ||
		header.Get("Cache-Control") != "private, no-store" {
		return fmt.Errorf("the download answered %s, want nosniff, a sandbox and no store", r.downloaded())
	}
	return nil
}

// downloadShownInline fails unless the last download is shown inline.
func (r *readScenario) downloadShownInline() error {
	if r.download.status != http.StatusOK || r.download.header.Get("Content-Disposition") != "inline" {
		return fmt.Errorf("the download answered %s, want 200 shown inline", r.downloaded())
	}
	return nil
}

// downloadStartsWith fails unless the last download succeeded holding bytes that start with prefix.
func (r *readScenario) downloadStartsWith(prefix string) error {
	if r.download.status != http.StatusOK || !bytes.HasPrefix(r.download.body, []byte(prefix)) {
		return fmt.Errorf("the download answered %s, want 200 starting with %q", r.downloaded(), prefix)
	}
	return nil
}

// downloadHoldsFedMail fails unless the last download succeeded holding exactly a mail the scenario fed.
func (r *readScenario) downloadHoldsFedMail() error {
	if r.download.status != http.StatusOK || !slices.Contains(r.fed, string(r.download.body)) {
		return fmt.Errorf("the download answered %s, want 200 holding a mail fed", r.downloaded())
	}
	return nil
}

// downloadNotFound fails unless the last download answered not found.
func (r *readScenario) downloadNotFound() error {
	return r.wantDownloadStatus(http.StatusNotFound)
}

// downloadForbidden fails unless the last download answered forbidden.
func (r *readScenario) downloadForbidden() error {
	return r.wantDownloadStatus(http.StatusForbidden)
}

// wantDownloadStatus fails unless the last download answered status.
func (r *readScenario) wantDownloadStatus(status int) error {
	if r.download.status != status {
		return fmt.Errorf("the download answered %s, want %d", r.downloaded(), status)
	}
	return nil
}

// listedThreads answers the subjects of threads as one comma list.
func listedThreads(threads []servedThread) string {
	subjects := make([]string, len(threads))
	for i, thread := range threads {
		subjects[i] = thread.Subject
	}
	return strings.Join(subjects, ", ")
}

// listedMails answers the subjects of mails as one comma list.
func listedMails(mails []servedMail) string {
	subjects := make([]string, len(mails))
	for i, mail := range mails {
		subjects[i] = mail.Subject
	}
	return strings.Join(subjects, ", ")
}

// shownPath answers path as text, no path when it is nil.
func shownPath(path *string) string {
	if path == nil {
		return "no path"
	}
	return *path
}

// quoted answers body cut to answerPreview bytes, for a failing step to quote.
func quoted(body []byte) string {
	return string(body[:min(len(body), answerPreview)])
}

func TestEmailRead(t *testing.T) {
	runFeature(t, emailReadFeature, initializeEmailRead(t))
}

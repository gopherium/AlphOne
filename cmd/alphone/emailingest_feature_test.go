// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
)

// emailIngestFeature is the feature storing mail on its known contacts answers to.
const emailIngestFeature = "../../test/features/features/email-ingest.feature"

// emailThreadsFeature is the feature threading stored mail answers to.
const emailThreadsFeature = "../../test/features/features/email-threads.feature"

// emailStorageFeature is the feature the files of stored mail answer to.
const emailStorageFeature = "../../test/features/features/email-storage.feature"

// emailPluginID is the id the email plugin keeps its files and settings under.
const emailPluginID = "email"

// mailboxAddress is the address of the mailbox every scenario feeds.
const mailboxAddress = "info@example.com"

// mailboxAlias is the alias of the mailbox every scenario feeds.
const mailboxAlias = "sales@example.com"

// emailChannel is the channel a contact is known on by an email address.
const emailChannel contact.Channel = "email"

// workspaceLimitKey is the plugin setting that holds a workspace's own mail limit.
const workspaceLimitKey = "workspace_max_bytes"

// sweepIntervalSetting names the setting that spaces the sweeps of the email plugin.
const sweepIntervalSetting = "ALPHONE_EMAIL_SWEEP_INTERVAL"

// rawMailType is the media type a raw copy of a mail is kept as.
const rawMailType = "message/rfc822"

// listenerURL is where the webhook a scenario registers would deliver.
const listenerURL = "https://hooks.example.com/email"

// listenerSecret is the secret of the webhook a scenario registers.
const listenerSecret = "whsec_email_scenario"

// serveStartBudget bounds how long a scenario waits for AlphOne to answer.
const serveStartBudget = 10 * time.Second

// serveStopBudget bounds how long a scenario waits for AlphOne to stop serving.
const serveStopBudget = 10 * time.Second

// probePause spaces two reads of a scenario waiting on AlphOne.
const probePause = 50 * time.Millisecond

// storeWorkspace stores one workspace.
const storeWorkspace = "INSERT INTO core.tenants (id, name) VALUES ($1, $2)"

// deactivateWorkspace marks one workspace deactivated a day before now.
const deactivateWorkspace = "UPDATE core.tenants SET deactivated_at = now() - interval '1 day' WHERE id = $1"

// accountIDAt reads the id of the account at an address.
const accountIDAt = "SELECT id FROM auth.users WHERE email = $1"

// contactNamed reads the id of the contact a workspace holds under a name.
const contactNamed = "SELECT id FROM core.contacts WHERE tenant_id = $1 AND name = $2"

// workspaceMails counts the mails a workspace stores.
const workspaceMails = "SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1"

// workspaceThreads counts the threads a workspace holds.
const workspaceThreads = "SELECT count(*) FROM plugin_email.threads WHERE tenant_id = $1"

// workspaceMailboxes counts the mailboxes a workspace holds.
const workspaceMailboxes = "SELECT count(*) FROM plugin_email.mailboxes WHERE tenant_id = $1"

// workspaceMailRows counts every row the mail of a workspace leaves in the tables of the email plugin.
const workspaceMailRows = `SELECT
	(SELECT count(*) FROM plugin_email.threads WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.provider_refs WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.thread_keys WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.message_contacts WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.attachments WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1) +
	(SELECT count(*) FROM plugin_email.failed_mail WHERE tenant_id = $1)`

// workspaceFiles counts the files kept under a key prefix.
const workspaceFiles = "SELECT count(*) FROM storage.objects WHERE convert_from(key, 'UTF8') LIKE $1"

// storedFile reads the media type and the bytes of the live file kept under a key.
const storedFile = `SELECT o.content_type, coalesce(string_agg(c.data, ''::bytea ORDER BY c.seq), ''::bytea)
	FROM storage.objects o LEFT JOIN storage.chunks c ON c.object_id = o.id
	WHERE convert_from(o.key, 'UTF8') = $1 AND o.state = 'live'
	GROUP BY o.id, o.content_type`

// titledMail counts the stored mails carrying a subject and reads the id of one of them.
const titledMail = "SELECT count(*), coalesce(min(id::text), '') FROM plugin_email.messages WHERE subject = $1"

// mailRead reads the columns of one stored mail the scenarios check.
const mailRead = `SELECT tenant_id, thread_id, direction, body_state, coalesce(body_text, ''),
	coalesce(safe_html, ''), raw_state, raw_file_id, skipped_parts, coalesce(invite_method, ''),
	participants::text
	FROM plugin_email.messages WHERE id = $1`

// mailFile reads the media type, the state and the file id of the attachment of one mail with a name.
const mailFile = `SELECT content_type, state, file_id FROM plugin_email.attachments
	WHERE message_id = $1 AND filename = $2`

// providerRefs counts the provider references of one mail.
const providerRefs = "SELECT count(*) FROM plugin_email.provider_refs WHERE message_id = $1"

// linkedContacts reads the ids of the contacts one mail is linked to, in the order of their text.
const linkedContacts = `SELECT contact_id::text FROM plugin_email.message_contacts
	WHERE message_id = $1 ORDER BY contact_id::text`

// contactMails counts the mails linked to one contact.
const contactMails = "SELECT count(*) FROM plugin_email.message_contacts WHERE contact_id = $1"

// contactHoldsMail reports whether one contact is linked to one mail.
const contactHoldsMail = `SELECT EXISTS (SELECT FROM plugin_email.message_contacts
	WHERE contact_id = $1 AND message_id = $2)`

// contactsInOneThread reports whether two contacts are linked to mails of one thread.
const contactsInOneThread = `SELECT EXISTS (SELECT FROM plugin_email.message_contacts a
	JOIN plugin_email.messages ma ON ma.id = a.message_id
	JOIN plugin_email.messages mb ON mb.thread_id = ma.thread_id
	JOIN plugin_email.message_contacts b ON b.message_id = mb.id
	WHERE a.contact_id = $1 AND b.contact_id = $2)`

// threadSubjects reads the subjects of the mails of one thread in the order they were received.
const threadSubjects = `SELECT string_agg(subject, ', ' ORDER BY received_at, id)
	FROM plugin_email.messages WHERE thread_id = $1`

// usageCountsText reports whether the usage of a workspace counts its kept files and the text of its mail.
const usageCountsText = `SELECT u.used_bytes >=
	coalesce((SELECT sum(raw_size) FROM plugin_email.messages
		WHERE tenant_id = $1 AND raw_file_id IS NOT NULL), 0) +
	coalesce((SELECT sum(size) FROM plugin_email.attachments
		WHERE tenant_id = $1 AND file_id IS NOT NULL), 0) +
	coalesce((SELECT sum(octet_length(body_text)) FROM plugin_email.messages WHERE tenant_id = $1), 0)
	FROM plugin_email.workspace_usage u WHERE u.tenant_id = $1`

// queuedEvents reads the payload of every delivery queued for an event.
const queuedEvents = "SELECT payload FROM core.webhook_deliveries WHERE event_name = $1"

// queuedFiles counts the files the queue of a workspace holds.
const queuedFiles = "SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1"

// filesBeingWritten counts the files the queue of a workspace holds as still being written.
const filesBeingWritten = "SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1 AND state = 'writing'"

// ageQueuedFiles moves every queued file of a workspace two days into the past.
const ageQueuedFiles = `UPDATE plugin_email.pending_files SET created_at = created_at - interval '2 days'
	WHERE tenant_id = $1`

// refuseMail makes the mail table refuse every new row.
const refuseMail = "ALTER TABLE plugin_email.messages ADD CONSTRAINT stopped_ingest CHECK (false) NOT VALID"

// allowMail takes back what refuseMail added.
const allowMail = "ALTER TABLE plugin_email.messages DROP CONSTRAINT stopped_ingest"

// refuseQueueing makes the file queue refuse a file on its way out.
const refuseQueueing = `ALTER TABLE plugin_email.pending_files
	ADD CONSTRAINT stopped_ingest CHECK (state <> 'deleting') NOT VALID`

// allowQueueing takes back what refuseQueueing added.
const allowQueueing = "ALTER TABLE plugin_email.pending_files DROP CONSTRAINT stopped_ingest"

// dryRunNotice is the line a command prints when it only previewed.
const dryRunNotice = "dry run, nothing changed, pass %s to apply"

// firstReceived is the time the first mail of a scenario is received at.
var firstReceived = time.Date(2026, time.November, 2, 9, 0, 0, 0, time.UTC)

// skippedStates names the state a kept file takes for each reason a scenario gives for skipping it.
var skippedStates = map[string]string{"too large": "too_large", "over the workspace limit": "over_limit"}

// emailScenario is what one email scenario keeps between its steps.
type emailScenario struct {
	t           *testing.T
	env         map[string]string
	pool        *pgxpool.Pool
	workspaces  map[string]uuid.UUID
	operator    string
	received    time.Time
	fed         []string
	result      testkit.Result
	sweep       time.Duration
	servedAt    time.Time
	stopServing context.CancelFunc
	served      chan error
}

// storedMail is one stored mail as the scenarios read it.
type storedMail struct {
	id           uuid.UUID
	tenant       uuid.UUID
	thread       uuid.UUID
	direction    string
	bodyState    string
	text         string
	html         string
	rawState     string
	rawFile      *uuid.UUID
	skippedParts int
	inviteMethod string
	participants string
}

// initializeEmail returns the binding of the email steps, t holding their databases.
func initializeEmail(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &emailScenario{t: t, env: map[string]string{}, workspaces: map[string]uuid.UUID{}, received: firstReceived}
		s.bindSetup(sc)
		s.bindFeeds(sc)
		s.bindAnswers(sc)
		s.bindMails(sc)
		s.bindFiles(sc)
		s.bindThreads(sc)
		s.bindEvents(sc)
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			return ctx, s.stop()
		})
	}
}

// bindSetup binds the steps that prepare a database, its accounts, its workspaces and their settings.
func (s *emailScenario) bindSetup(sc *godog.ScenarioContext) {
	sc.Given(`^the database is migrated$`, s.migrateTheDatabase)
	sc.Given(`^the administrator "([^"]*)"$`, s.holdAdministrator)
	sc.Given(`^the member "([^"]*)"$`, s.holdMember)
	sc.Given(`^the workspace "([^"]*)" exists$`, s.createWorkspace)
	sc.Given(`^the workspace "([^"]*)" knows "([^"]*)" at "([^"]*)"$`, s.knowContact)
	sc.Given(`^the setting "([^"]*)" holds "([^"]*)"$`, s.holdSetting)
	sc.Given(`^the workspace "([^"]*)" holds its own mail limit "([^"]*)"$`, s.holdOwnLimit)
	sc.Given(`^the workspace "([^"]*)" was deactivated a day ago$`, s.deactivate)
	sc.Given(`^a webhook of "([^"]*)" listens for "([^"]*)"$`, s.listenFor)
	sc.Step(`^(\d+) days? pass(?:es)?$`, s.letDaysPass)
}

// bindFeeds binds the steps that feed raw mail through email:ingest.
func (s *emailScenario) bindFeeds(sc *godog.ScenarioContext) {
	sc.When(`^this mail reaches "([^"]*)":$`, s.reach)
	sc.Given(`^this mail reached "([^"]*)":$`, s.reachOrFail)
	sc.When(`^this Sent copy reaches "([^"]*)":$`, s.reachAsSentCopy)
	sc.Given(`^this Sent copy reached "([^"]*)":$`, s.reachAsSentCopyOrFail)
	sc.When(`^this mail reaches "([^"]*)" in the provider thread "([^"]*)":$`, s.reachInProviderThread)
	sc.Given(`^this mail reached "([^"]*)" in the provider thread "([^"]*)":$`, s.reachInProviderThreadOrFail)
	sc.When(`^this mail is previewed for "([^"]*)":$`, s.preview)
	sc.When(`^the member "([^"]*)" feeds this mail to "([^"]*)":$`, s.feedAsMember)
	sc.Step(`^the same mail reaches "([^"]*)"(?: again)?$`, s.reachAgain)
	sc.When(`^the previewed mail reaches "([^"]*)"$`, s.reachAgain)
	sc.When(`^a mail titled "([^"]*)" reaches "([^"]*)" from a sender whose name holds a raw Latin-1 byte$`,
		s.reachFromLatin1Sender)
	sc.When(`^a mail titled "([^"]*)" reaches "([^"]*)" holding (\d+) attached messages of (\d+) levels each$`,
		s.reachWithNestedMessages)
	sc.Given(`^an ingest of this mail into "([^"]*)" stopped two days ago after it kept (\d+) files:$`,
		s.stopIngest)
}

// bindAnswers binds the steps that read what email:ingest answered.
func (s *emailScenario) bindAnswers(sc *godog.ScenarioContext) {
	sc.Then(`^the command answers "([^"]*)"$`, s.answers)
	sc.Then(`^the command exits with code (\d+)$`, s.exitsWith)
	sc.Then(`^the error says "([^"]*)"$`, s.stderrCarries)
	sc.Then(`^the command warns about "([^"]*)"$`, s.stderrCarries)
	sc.Then(`^the answer says nothing changed until it is confirmed with "([^"]*)"$`, s.saysNothingChanged)
}

// bindMails binds the steps that read the stored mail and its contacts.
func (s *emailScenario) bindMails(sc *godog.ScenarioContext) {
	sc.Step(`^the workspace "([^"]*)" holds (\d+) mails?$`, s.holdsMails)
	sc.Then(`^the workspace "([^"]*)" holds no mailbox$`, s.holdsNoMailbox)
	sc.Then(`^the workspace "([^"]*)" keeps no mail and no file$`, s.keepsNoMail)
	sc.Then(`^the contact "([^"]*)" in "([^"]*)" holds the mail "([^"]*)"$`, s.contactHolds)
	sc.Then(`^the contact "([^"]*)" in "([^"]*)" holds no mail$`, s.contactHoldsNoMail)
	sc.Step(`^the mail "([^"]*)" is (inbound|outbound)$`, s.wantDirection)
	sc.Then(`^the mail "([^"]*)" keeps the text "([^"]*)"$`, s.keepsText)
	sc.Then(`^the mail "([^"]*)" keeps its whole text$`, s.keepsWholeText)
	sc.Then(`^the mail "([^"]*)" is stored cut with the text "([^"]*)"$`, s.wantBodyCut)
	sc.Then(`^the mail "([^"]*)" is stored whole$`, s.wantBodyWhole)
	sc.Then(`^the mail "([^"]*)" keeps its headers only$`, s.keepsHeadersOnly)
	sc.Then(`^the mail "([^"]*)" names the address "([^"]*)"$`, s.namesAddress)
	sc.Then(`^the mail "([^"]*)" names no address "([^"]*)"$`, s.namesNoAddress)
	sc.Then(`^the mail "([^"]*)" holds no invite method$`, s.holdsNoInviteMethod)
	sc.Then(`^the mail "([^"]*)" counts (\d+) skipped parts?$`, s.countsSkippedParts)
	sc.Then(`^the mail "([^"]*)" has (\d+) provider references?$`, s.wantProviderRefs)
}

// bindFiles binds the steps that read the files of stored mail and run the sweep.
func (s *emailScenario) bindFiles(sc *godog.ScenarioContext) {
	sc.Step(`^the mail "([^"]*)" keeps its raw copy$`, s.keepsRawCopy)
	sc.Then(`^the mail "([^"]*)" keeps no raw copy as it is (too large|over the workspace limit)$`, s.keepsNoRawCopy)
	sc.Then(`^the mail "([^"]*)" keeps the file "([^"]*)" as "([^"]*)"$`, s.keepsFile)
	sc.Then(`^the mail "([^"]*)" names the file "([^"]*)" with no copy as it is `+
		`(too large|over the workspace limit)$`, s.namesFileWithNoCopy)
	sc.Then(`^the workspace "([^"]*)" keeps (\d+) files?$`, s.keepsFiles)
	sc.Then(`^the workspace "([^"]*)" keeps no file$`, s.keepsNoFile)
	sc.Step(`^the usage of "([^"]*)" counts its stored files and its stored text$`, s.countsStoredText)
	sc.When(`^AlphOne serves with an email sweep every (\d+) seconds$`, s.serveWithSweep)
	sc.Then(`^the file queue of "([^"]*)" is empty after one sweep$`, s.queueEmptiesInOneSweep)
}

// bindThreads binds the steps that read the threads of stored mail.
func (s *emailScenario) bindThreads(sc *godog.ScenarioContext) {
	sc.Step(`^the workspace "([^"]*)" holds (\d+) threads?$`, s.holdsThreads)
	sc.Then(`^the mails "([^"]*)" and "([^"]*)" share a thread$`, s.shareAThread)
	sc.Then(`^the mails "([^"]*)" and "([^"]*)" do not share a thread$`, s.shareNoThread)
	sc.Then(`^the contacts "([^"]*)" and "([^"]*)" in "([^"]*)" share no thread$`, s.contactsShareNoThread)
	sc.Then(`^the thread of "([^"]*)" lists "([^"]*)" in time order$`, s.threadLists)
}

// bindEvents binds the steps that read the events stored mail queued for webhooks.
func (s *emailScenario) bindEvents(sc *godog.ScenarioContext) {
	sc.Then(`^no "([^"]*)" event is queued$`, s.queuesNoEvent)
	sc.Then(`^the event "([^"]*)" carries the id, thread, contacts, subject and text of the mail "([^"]*)" `+
		`and nothing else$`, s.eventCarriesTheMail)
	sc.Then(`^the event "([^"]*)" carries the text "([^"]*)"$`, s.eventCarriesText)
}

// migrateTheDatabase points the settings at a fresh database that alphone migrate brought up to date.
func (s *emailScenario) migrateTheDatabase(ctx context.Context) error {
	s.env["ALPHONE_DATABASE_URL"] = barePostgres(s.t)
	if got := s.answer("", "migrate"); got.Code != gonsole.ExitDone {
		return fmt.Errorf("migrate exited with %d and stderr %q, want 0", got.Code, got.Stderr)
	}
	pool, err := pgxpool.New(ctx, s.env["ALPHONE_DATABASE_URL"])
	if err != nil {
		return err
	}
	s.pool = pool
	return nil
}

// holdAccount creates the account at email under the role, typing the password on standard input.
func (s *emailScenario) holdAccount(email string, held role.Role) error {
	got := s.answer(typedPassword+"\n", "account:create-admin", "-email", email, "-name", "Holder", "-role",
		held.String())
	if got.Code != gonsole.ExitDone {
		return fmt.Errorf("account:create-admin for %s exited with %d and stderr %q, want 0", email, got.Code, got.Stderr)
	}
	return nil
}

// holdAdministrator creates the administrator at email and feeds every later mail as that account.
func (s *emailScenario) holdAdministrator(email string) error {
	s.operator = email
	return s.holdAccount(email, role.Admin)
}

// holdMember creates the member at email.
func (s *emailScenario) holdMember(email string) error {
	return s.holdAccount(email, role.Member)
}

// createWorkspace stores the workspace called name and keeps its id.
func (s *emailScenario) createWorkspace(ctx context.Context, name string) error {
	id := uuid.Must(uuid.NewV7())
	if _, err := s.pool.Exec(ctx, storeWorkspace, id, name); err != nil {
		return fmt.Errorf("storing the workspace %s: %w", name, err)
	}
	s.workspaces[name] = id
	return nil
}

// in returns ctx serving the workspace called name.
func (s *emailScenario) in(ctx context.Context, name string) context.Context {
	return sdk.WithTenant(ctx, s.workspaces[name])
}

// knowContact stores the contact called name in the workspace, known by the email address.
func (s *emailScenario) knowContact(ctx context.Context, workspace, name, address string) error {
	resolver := contact.NewResolver(postgres.NewContactStore(s.pool))
	if _, err := resolver.Resolve(s.in(ctx, workspace), emailChannel, address, name); err != nil {
		return fmt.Errorf("storing the contact %s in %s: %w", name, workspace, err)
	}
	return nil
}

// holdSetting gives the setting called key the value.
func (s *emailScenario) holdSetting(key, value string) {
	s.env[key] = value
}

// holdOwnLimit stores limit as the workspace's own mail limit through the host's settings store.
func (s *emailScenario) holdOwnLimit(ctx context.Context, workspace, limit string) error {
	store := postgres.NewSettingStore(s.pool, nil).ForPlugin(emailPluginID)
	return store.Save(s.in(ctx, workspace), map[string]string{workspaceLimitKey: limit})
}

// deactivate marks the workspace called name deactivated a day ago.
func (s *emailScenario) deactivate(ctx context.Context, name string) error {
	_, err := s.pool.Exec(ctx, deactivateWorkspace, s.workspaces[name])
	return err
}

// listenFor registers a webhook of the operator in the workspace for the event called name.
func (s *emailScenario) listenFor(ctx context.Context, workspace, name string) error {
	var owner uuid.UUID
	if err := s.pool.QueryRow(ctx, accountIDAt, s.operator).Scan(&owner); err != nil {
		return fmt.Errorf("reading the account %s: %w", s.operator, err)
	}
	return postgres.NewWebhookStore(s.pool).CreateSubscription(s.in(ctx, workspace), webhook.Subscription{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    owner,
		URL:       listenerURL,
		Events:    []event.Name{event.Name(name)},
		Secret:    listenerSecret,
		CreatedAt: time.Now(),
	})
}

// letDaysPass moves the time the next mail is received at the given number of days on.
func (s *emailScenario) letDaysPass(days int) {
	s.received = s.received.Add(time.Duration(days) * 24 * time.Hour)
}

// answer runs the compiled command line over the scenario's settings, feeding stdin.
func (s *emailScenario) answer(stdin string, args ...string) testkit.Result {
	return testkit.Run(s.t, compiledProgram(s.env), stdin, args...)
}

// rawMail answers the mail a doc string spells, every line ended the way mail ends it.
func rawMail(mail *godog.DocString) string {
	return strings.ReplaceAll(mail.Content, "\n", "\r\n") + "\r\n"
}

// feed runs email:ingest of raw for the workspace called name acting as actor, the flags after the received time.
func (s *emailScenario) feed(name, actor, raw string, flags ...string) {
	args := append([]string{
		"email:ingest", s.workspaces[name].String(), "-mailbox", mailboxAddress, "-alias", mailboxAlias,
		"-as", actor, "-received", s.received.Format(time.RFC3339),
	}, flags...)
	s.received = s.received.Add(time.Minute)
	s.fed = append(s.fed, raw)
	s.result = s.answer(raw, args...)
}

// wantExitZero fails unless the last command exited with code 0.
func (s *emailScenario) wantExitZero() error {
	return s.exitsWith(gonsole.ExitDone)
}

// reach feeds the mail to the workspace called name, confirmed with -yes.
func (s *emailScenario) reach(name string, mail *godog.DocString) {
	s.feed(name, s.operator, rawMail(mail), "-yes")
}

// reachOrFail feeds the mail to the workspace called name, confirmed with -yes, and fails unless it exits 0.
func (s *emailScenario) reachOrFail(name string, mail *godog.DocString) error {
	s.reach(name, mail)
	return s.wantExitZero()
}

// reachAsSentCopy feeds the mail to the workspace called name as a copy read from the Sent folder.
func (s *emailScenario) reachAsSentCopy(name string, mail *godog.DocString) {
	s.feed(name, s.operator, rawMail(mail), "-sent-copy", "-yes")
}

// reachAsSentCopyOrFail feeds the mail as a copy from the Sent folder and fails unless it exits 0.
func (s *emailScenario) reachAsSentCopyOrFail(name string, mail *godog.DocString) error {
	s.reachAsSentCopy(name, mail)
	return s.wantExitZero()
}

// reachInProviderThread feeds the mail to the workspace called name under the provider's thread id.
func (s *emailScenario) reachInProviderThread(name, thread string, mail *godog.DocString) {
	s.feed(name, s.operator, rawMail(mail), "-provider-thread", thread, "-yes")
}

// reachInProviderThreadOrFail feeds the mail under the provider's thread id and fails unless it exits 0.
func (s *emailScenario) reachInProviderThreadOrFail(name, thread string, mail *godog.DocString) error {
	s.reachInProviderThread(name, thread, mail)
	return s.wantExitZero()
}

// preview feeds the mail to the workspace called name with no -yes.
func (s *emailScenario) preview(name string, mail *godog.DocString) {
	s.feed(name, s.operator, rawMail(mail))
}

// feedAsMember feeds the mail to the workspace called name acting as the member at email.
func (s *emailScenario) feedAsMember(email, name string, mail *godog.DocString) {
	s.feed(name, email, rawMail(mail), "-yes")
}

// reachAgain feeds the last mail fed once more to the workspace called name, confirmed with -yes.
func (s *emailScenario) reachAgain(name string) error {
	if len(s.fed) == 0 {
		return errors.New("no mail was fed before, want one to feed again")
	}
	s.feed(name, s.operator, s.fed[len(s.fed)-1], "-yes")
	return nil
}

// reachFromLatin1Sender feeds a mail titled subject whose sender name holds the raw Latin-1 byte 0xA9.
func (s *emailScenario) reachFromLatin1Sender(subject, name string) {
	s.feed(name, s.operator, latin1SenderMail(subject), "-yes")
}

// reachWithNestedMessages feeds a mail titled subject holding count attached messages nested levels deep.
func (s *emailScenario) reachWithNestedMessages(subject, name string, count, levels int) {
	s.feed(name, s.operator, nestedMessagesMail(subject, count, levels), "-yes")
}

// latin1SenderMail answers a raw mail titled subject from a sender whose name holds the raw byte 0xA9.
func latin1SenderMail(subject string) string {
	return "From: Maria Perez \xa9 Lamps <maria.perez@example.com>\r\n" +
		"To: info@example.com\r\n" +
		"Subject: " + subject + "\r\n" +
		"Date: Mon, 02 Nov 2026 09:00:00 +0000\r\n" +
		"Message-ID: <latin1-sender@example.com>\r\n" +
		"Content-Type: text/plain; charset=iso-8859-1\r\n\r\n" +
		"The lamp catalogue is ready.\r\n"
}

// nestedMessagesMail answers a raw mail titled subject holding count attached messages nested levels deep.
func nestedMessagesMail(subject string, count, levels int) string {
	var parts strings.Builder
	for index := range count {
		_, _ = fmt.Fprintf(&parts, "--outer\r\nContent-Type: %s\r\n"+
			"Content-Disposition: attachment; filename=\"thread-%d.eml\"\r\n\r\n%s\r\n",
			rawMailType, index+1, nestedMessage(index+1, levels))
	}
	return "From: Maria Perez <maria.perez@example.com>\r\nTo: info@example.com\r\nSubject: " + subject + "\r\n" +
		"Date: Mon, 02 Nov 2026 09:00:00 +0000\r\nMessage-ID: <nested-messages@example.com>\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"outer\"\r\n\r\n" +
		"--outer\r\nContent-Type: text/plain\r\n\r\nThe old threads are attached.\r\n" + parts.String() + "--outer--\r\n"
}

// nestedMessage answers the raw message attached at index, its text inside levels nested containers.
func nestedMessage(index, levels int) string {
	body := "Content-Type: text/plain\r\n\r\nThe oldest note.\r\n"
	for level := levels; level > 0; level-- {
		boundary := fmt.Sprintf("thread-%d-level-%d", index, level)
		body = fmt.Sprintf("Content-Type: multipart/mixed; boundary=%q\r\n\r\n--%s\r\n%s--%s--\r\n",
			boundary, boundary, body, boundary)
	}
	return fmt.Sprintf("From: Maria Perez <maria.perez@example.com>\r\nSubject: Old thread %d\r\n%s", index, body)
}

// stopIngest feeds the mail to the workspace with its commit and clean up refused, then ages what it left behind.
func (s *emailScenario) stopIngest(ctx context.Context, workspace string, files int, mail *godog.DocString) error {
	if err := s.execEach(ctx, refuseMail, refuseQueueing); err != nil {
		return fmt.Errorf("stopping the ingest before its commit: %w", err)
	}
	s.feed(workspace, s.operator, rawMail(mail), "-yes")
	if err := s.execEach(ctx, allowMail, allowQueueing); err != nil {
		return fmt.Errorf("letting the mail through again: %w", err)
	}
	if err := s.wantFilesBeingWritten(ctx, workspace, files); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, ageQueuedFiles, s.workspaces[workspace])
	return err
}

// execEach runs every statement on the scenario's pool in order, stopping at the first failure.
func (s *emailScenario) execEach(ctx context.Context, statements ...string) error {
	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// wantFilesBeingWritten fails unless the queue of the workspace holds count files still being written, each one kept.
func (s *emailScenario) wantFilesBeingWritten(ctx context.Context, workspace string, count int) error {
	writing, err := s.count(ctx, filesBeingWritten, s.workspaces[workspace])
	if err != nil {
		return err
	}
	kept, err := s.filesOf(ctx, workspace)
	if err != nil {
		return err
	}
	if writing != count || kept != count {
		return fmt.Errorf("the stopped ingest left %d files being written and kept %d (stdout %q, stderr %q), "+
			"want %d of each", writing, kept, s.result.Stdout, s.result.Stderr, count)
	}
	return nil
}

// answers fails unless the last command names outcome as a word of its own.
func (s *emailScenario) answers(outcome string) error {
	named := regexp.MustCompile(`(^|[^a-z_])` + regexp.QuoteMeta(outcome) + `($|[^a-z_])`)
	if !named.MatchString(s.result.Stdout + "\n" + s.result.Stderr) {
		return fmt.Errorf("the command exited with %d, stdout %q, stderr %q, want the outcome %s",
			s.result.Code, s.result.Stdout, s.result.Stderr, outcome)
	}
	return nil
}

// exitsWith fails unless the last command exited with code.
func (s *emailScenario) exitsWith(code int) error {
	if s.result.Code != code {
		return fmt.Errorf("the command exited with %d, stdout %q, stderr %q, want %d",
			s.result.Code, s.result.Stdout, s.result.Stderr, code)
	}
	return nil
}

// stderrCarries fails unless the last command wrote text on stderr.
func (s *emailScenario) stderrCarries(text string) error {
	if !strings.Contains(s.result.Stderr, text) {
		return fmt.Errorf("stderr %q does not carry %q", s.result.Stderr, text)
	}
	return nil
}

// saysNothingChanged fails unless the last command reported a dry run that the flag would apply.
func (s *emailScenario) saysNothingChanged(flag string) error {
	return s.stderrCarries(fmt.Sprintf(dryRunNotice, flag))
}

// count reads the one count query answers for args.
func (s *emailScenario) count(ctx context.Context, query string, args ...any) (int, error) {
	var counted int
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&counted); err != nil {
		return 0, err
	}
	return counted, nil
}

// wantCount fails unless query counts want for the workspace called name, naming what it counts.
func (s *emailScenario) wantCount(ctx context.Context, query, what, name string, want int) error {
	held, err := s.count(ctx, query, s.workspaces[name])
	if err != nil {
		return fmt.Errorf("counting the %s of %s: %w", what, name, err)
	}
	if held != want {
		return fmt.Errorf("the workspace %s holds %d %s, want %d", name, held, what, want)
	}
	return nil
}

// holdsMails fails unless the workspace called name stores want mails.
func (s *emailScenario) holdsMails(ctx context.Context, name string, want int) error {
	return s.wantCount(ctx, workspaceMails, "mails", name, want)
}

// holdsThreads fails unless the workspace called name holds want threads.
func (s *emailScenario) holdsThreads(ctx context.Context, name string, want int) error {
	return s.wantCount(ctx, workspaceThreads, "threads", name, want)
}

// holdsNoMailbox fails unless the workspace called name holds no mailbox.
func (s *emailScenario) holdsNoMailbox(ctx context.Context, name string) error {
	return s.wantCount(ctx, workspaceMailboxes, "mailboxes", name, 0)
}

// keepsNoMail fails unless the workspace called name holds no row of any mail and keeps no file.
func (s *emailScenario) keepsNoMail(ctx context.Context, name string) error {
	if err := s.wantCount(ctx, workspaceMailRows, "rows of mail", name, 0); err != nil {
		return err
	}
	return s.keepsNoFile(ctx, name)
}

// filesOf counts the files the email plugin keeps for the workspace called name.
func (s *emailScenario) filesOf(ctx context.Context, name string) (int, error) {
	return s.count(ctx, workspaceFiles, "t/"+s.workspaces[name].String()+"/"+emailPluginID+"/%")
}

// keepsFiles fails unless the email plugin keeps want files for the workspace called name.
func (s *emailScenario) keepsFiles(ctx context.Context, name string, want int) error {
	kept, err := s.filesOf(ctx, name)
	if err != nil {
		return fmt.Errorf("counting the files of %s: %w", name, err)
	}
	if kept != want {
		return fmt.Errorf("the workspace %s keeps %d files, want %d", name, kept, want)
	}
	return nil
}

// keepsNoFile fails unless the email plugin keeps no file for the workspace called name.
func (s *emailScenario) keepsNoFile(ctx context.Context, name string) error {
	return s.keepsFiles(ctx, name, 0)
}

// contactID reads the id of the contact called name in the workspace.
func (s *emailScenario) contactID(ctx context.Context, name, workspace string) (uuid.UUID, error) {
	var id uuid.UUID
	if err := s.pool.QueryRow(ctx, contactNamed, s.workspaces[workspace], name).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("reading the contact %s of %s: %w", name, workspace, err)
	}
	return id, nil
}

// readMailIDBySubject reads the id of the one stored mail whose subject is subject.
func (s *emailScenario) readMailIDBySubject(ctx context.Context, subject string) (uuid.UUID, error) {
	var held int
	var id string
	if err := s.pool.QueryRow(ctx, titledMail, subject).Scan(&held, &id); err != nil {
		return uuid.Nil, fmt.Errorf("reading the mail %q: %w", subject, err)
	}
	if held != 1 {
		return uuid.Nil, fmt.Errorf("%d stored mails are titled %q, want 1", held, subject)
	}
	return uuid.Parse(id)
}

// mail reads the stored mail whose subject is subject.
func (s *emailScenario) mail(ctx context.Context, subject string) (storedMail, error) {
	id, err := s.readMailIDBySubject(ctx, subject)
	if err != nil {
		return storedMail{}, err
	}
	held := storedMail{id: id}
	err = s.pool.QueryRow(ctx, mailRead, id).Scan(&held.tenant, &held.thread, &held.direction, &held.bodyState,
		&held.text, &held.html, &held.rawState, &held.rawFile, &held.skippedParts, &held.inviteMethod,
		&held.participants)
	if err != nil {
		return storedMail{}, fmt.Errorf("reading the mail %q: %w", subject, err)
	}
	return held, nil
}

// checkMail reads the mail titled subject and fails with what check finds wrong with it.
func (s *emailScenario) checkMail(ctx context.Context, subject string, check func(storedMail) error) error {
	held, err := s.mail(ctx, subject)
	if err != nil {
		return err
	}
	if err := check(held); err != nil {
		return fmt.Errorf("the mail %q %w", subject, err)
	}
	return nil
}

// contactHolds fails unless the contact called name in the workspace is linked to the mail titled subject.
func (s *emailScenario) contactHolds(ctx context.Context, name, workspace, subject string) error {
	contactID, err := s.contactID(ctx, name, workspace)
	if err != nil {
		return err
	}
	mailID, err := s.readMailIDBySubject(ctx, subject)
	if err != nil {
		return err
	}
	var linked bool
	if err := s.pool.QueryRow(ctx, contactHoldsMail, contactID, mailID).Scan(&linked); err != nil || !linked {
		return fmt.Errorf("the contact %s holds the mail %q %v (%v), want it linked", name, subject, linked, err)
	}
	return nil
}

// contactHoldsNoMail fails unless the contact called name in the workspace is linked to no mail.
func (s *emailScenario) contactHoldsNoMail(ctx context.Context, name, workspace string) error {
	contactID, err := s.contactID(ctx, name, workspace)
	if err != nil {
		return err
	}
	held, err := s.count(ctx, contactMails, contactID)
	if err != nil || held != 0 {
		return fmt.Errorf("the contact %s holds %d mails (%v), want none", name, held, err)
	}
	return nil
}

// wantDirection fails unless the mail titled subject is stored as direction.
func (s *emailScenario) wantDirection(ctx context.Context, subject, direction string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if held.direction != direction {
			return fmt.Errorf("is %s, want %s", held.direction, direction)
		}
		return nil
	})
}

// keepsText fails unless the mail titled subject keeps text as its body, blanks around it aside.
func (s *emailScenario) keepsText(ctx context.Context, subject, text string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if strings.TrimSpace(held.text) != text {
			return fmt.Errorf("keeps the text %q, want %q", held.text, text)
		}
		return nil
	})
}

// keepsWholeText fails unless the mail titled subject keeps a text it did not cut.
func (s *emailScenario) keepsWholeText(ctx context.Context, subject string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if held.bodyState != "whole" || strings.TrimSpace(held.text) == "" {
			return fmt.Errorf("keeps the text %q as %s, want a whole text", held.text, held.bodyState)
		}
		return nil
	})
}

// wantBodyCut fails unless the mail titled subject keeps a body cut to text.
func (s *emailScenario) wantBodyCut(ctx context.Context, subject, text string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if held.bodyState != "cut" || strings.TrimSpace(held.text) != text {
			return fmt.Errorf("keeps the text %q as %s, want %q cut", held.text, held.bodyState, text)
		}
		return nil
	})
}

// wantBodyWhole fails unless the mail titled subject keeps its whole body.
func (s *emailScenario) wantBodyWhole(ctx context.Context, subject string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if held.bodyState != "whole" {
			return fmt.Errorf("keeps its body as %s, want it whole", held.bodyState)
		}
		return nil
	})
}

// keepsHeadersOnly fails unless the mail titled subject keeps its headers and no body.
func (s *emailScenario) keepsHeadersOnly(ctx context.Context, subject string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if held.bodyState != "headers_only" || held.text != "" || held.html != "" {
			return fmt.Errorf("keeps its body as %s with the text %q and the HTML %q, want its headers only",
				held.bodyState, held.text, held.html)
		}
		return nil
	})
}

// namesAddress fails unless the people on the mail titled subject name address.
func (s *emailScenario) namesAddress(ctx context.Context, subject, address string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if !strings.Contains(held.participants, address) {
			return fmt.Errorf("names %s, want %s among them", held.participants, address)
		}
		return nil
	})
}

// namesNoAddress fails when the people on the mail titled subject name address.
func (s *emailScenario) namesNoAddress(ctx context.Context, subject, address string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if strings.Contains(held.participants, address) {
			return fmt.Errorf("names %s, want %s left out", held.participants, address)
		}
		return nil
	})
}

// holdsNoInviteMethod fails unless the mail titled subject holds no calendar method.
func (s *emailScenario) holdsNoInviteMethod(ctx context.Context, subject string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if held.inviteMethod != "" {
			return fmt.Errorf("holds the invite method %q, want none", held.inviteMethod)
		}
		return nil
	})
}

// countsSkippedParts fails unless the mail titled subject counts want parts it could not read.
func (s *emailScenario) countsSkippedParts(ctx context.Context, subject string, want int) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if held.skippedParts != want {
			return fmt.Errorf("counts %d skipped parts, want %d", held.skippedParts, want)
		}
		return nil
	})
}

// wantProviderRefs fails unless the mail titled subject holds want provider references.
func (s *emailScenario) wantProviderRefs(ctx context.Context, subject string, want int) error {
	id, err := s.readMailIDBySubject(ctx, subject)
	if err != nil {
		return err
	}
	held, err := s.count(ctx, providerRefs, id)
	if err != nil || held != want {
		return fmt.Errorf("the mail %q has %d provider references (%v), want %d", subject, held, err, want)
	}
	return nil
}

// fileKey answers the key the email plugin keeps the file id of a workspace under.
func fileKey(tenant, id uuid.UUID) string {
	return "t/" + tenant.String() + "/" + emailPluginID + "/" + id.String()
}

// storedFileOf reads the media type and the bytes of the file id the email plugin keeps for tenant.
func (s *emailScenario) storedFileOf(ctx context.Context, tenant, id uuid.UUID) (string, []byte, error) {
	var contentType string
	var body []byte
	if err := s.pool.QueryRow(ctx, storedFile, fileKey(tenant, id)).Scan(&contentType, &body); err != nil {
		return "", nil, fmt.Errorf("reading the file %s: %w", id, err)
	}
	return contentType, body, nil
}

// keepsRawCopy fails unless the mail titled subject keeps a raw copy holding exactly a mail the scenario fed.
func (s *emailScenario) keepsRawCopy(ctx context.Context, subject string) error {
	held, err := s.mail(ctx, subject)
	if err != nil {
		return err
	}
	if held.rawState != "stored" || held.rawFile == nil {
		return fmt.Errorf("the mail %q keeps its raw copy as %s with the file %v, want it stored",
			subject, held.rawState, held.rawFile)
	}
	contentType, body, err := s.storedFileOf(ctx, held.tenant, *held.rawFile)
	if err != nil {
		return err
	}
	if contentType != rawMailType || !slices.Contains(s.fed, string(body)) {
		return fmt.Errorf("the raw copy of %q is a %s file of %d bytes, want a %s file holding a mail fed",
			subject, contentType, len(body), rawMailType)
	}
	return nil
}

// keepsNoRawCopy fails unless the mail titled subject keeps no raw copy, for the reason given.
func (s *emailScenario) keepsNoRawCopy(ctx context.Context, subject, reason string) error {
	return s.checkMail(ctx, subject, func(held storedMail) error {
		if held.rawState != skippedStates[reason] || held.rawFile != nil {
			return fmt.Errorf("keeps its raw copy as %s with the file %v, want none as %s",
				held.rawState, held.rawFile, skippedStates[reason])
		}
		return nil
	})
}

// attachment reads the media type, the state and the file id of the file called filename of the mail titled subject.
func (s *emailScenario) attachment(ctx context.Context, subject, filename string) (string, string, *uuid.UUID, error) {
	id, err := s.readMailIDBySubject(ctx, subject)
	if err != nil {
		return "", "", nil, err
	}
	var contentType, state string
	var file *uuid.UUID
	if err := s.pool.QueryRow(ctx, mailFile, id, filename).Scan(&contentType, &state, &file); err != nil {
		return "", "", nil, fmt.Errorf("reading the file %s of %q: %w", filename, subject, err)
	}
	return contentType, state, file, nil
}

// keepsFile fails unless the mail titled subject keeps the file called filename stored as contentType.
func (s *emailScenario) keepsFile(ctx context.Context, subject, filename, contentType string) error {
	held, err := s.mail(ctx, subject)
	if err != nil {
		return err
	}
	stated, state, file, err := s.attachment(ctx, subject, filename)
	if err != nil {
		return err
	}
	if stated != contentType || state != "stored" || file == nil {
		return fmt.Errorf("the mail %q names the file %s as %s, %s, with the file %v, want %s and stored",
			subject, filename, stated, state, file, contentType)
	}
	kept, _, err := s.storedFileOf(ctx, held.tenant, *file)
	if err != nil {
		return err
	}
	if kept != contentType {
		return fmt.Errorf("the file %s of %q is kept as %s, want %s", filename, subject, kept, contentType)
	}
	return nil
}

// namesFileWithNoCopy fails unless the mail titled subject names the file called filename with no file, as reason says.
func (s *emailScenario) namesFileWithNoCopy(ctx context.Context, subject, filename, reason string) error {
	_, state, file, err := s.attachment(ctx, subject, filename)
	if err != nil {
		return err
	}
	if state != skippedStates[reason] || file != nil {
		return fmt.Errorf("the mail %q names the file %s as %s with the file %v, want no file as %s",
			subject, filename, state, file, skippedStates[reason])
	}
	return nil
}

// countsStoredText fails unless the usage of the workspace called name counts its kept files and its text.
func (s *emailScenario) countsStoredText(ctx context.Context, name string) error {
	var counted bool
	if err := s.pool.QueryRow(ctx, usageCountsText, s.workspaces[name]).Scan(&counted); err != nil || !counted {
		return fmt.Errorf("the usage of %s counts its files and its text %v (%v), want it to", name, counted, err)
	}
	return nil
}

// serveWithSweep starts AlphOne serving with a sweep of the email plugin every given number of seconds.
func (s *emailScenario) serveWithSweep(seconds int) error {
	s.sweep = time.Duration(seconds) * time.Second
	s.env[sweepIntervalSetting] = s.sweep.String()
	address := testkit.FreeAddr(s.t)
	s.env["ALPHONE_ADDR"] = address
	serving, cancel := context.WithCancel(s.t.Context())
	s.stopServing, s.served = cancel, make(chan error, 1)
	getenv := testkit.Getenv(maps.Clone(s.env))
	go func() { s.served <- run(serving, getenv, io.Discard, registerPlugins) }()
	if err := s.awaitServer(address); err != nil {
		return err
	}
	s.servedAt = time.Now()
	return nil
}

// awaitServer waits until AlphOne answers the version probe at address, an error once it stops or the budget ends.
func (s *emailScenario) awaitServer(address string) error {
	deadline := time.Now().Add(serveStartBudget)
	for time.Now().Before(deadline) {
		select {
		case err := <-s.served:
			s.stopServing = nil
			return fmt.Errorf("AlphOne stopped before it answered: %v", err)
		default:
		}
		if answersProbe(address) {
			return nil
		}
		time.Sleep(probePause)
	}
	return fmt.Errorf("AlphOne never answered at %s within %v", address, serveStartBudget)
}

// answersProbe reports whether the server at address answers the version probe.
func answersProbe(address string) bool {
	response, err := http.Post("http://"+address+"/api/graphql", "application/json", strings.NewReader(versionProbe))
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode == http.StatusOK
}

// queueEmptiesInOneSweep fails unless the file queue of the workspace called name empties within one sweep.
func (s *emailScenario) queueEmptiesInOneSweep(ctx context.Context, name string) error {
	deadline := s.servedAt.Add(s.sweep + s.sweep/2)
	for {
		queued, err := s.count(ctx, queuedFiles, s.workspaces[name])
		if err != nil {
			return err
		}
		if queued == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the file queue of %s holds %d files one sweep after AlphOne served, want none",
				name, queued)
		}
		time.Sleep(probePause)
	}
}

// threadOf reads the thread id of the mail titled subject.
func (s *emailScenario) threadOf(ctx context.Context, subject string) (uuid.UUID, error) {
	held, err := s.mail(ctx, subject)
	return held.thread, err
}

// threadsOf reads the thread ids of the mails titled first and second.
func (s *emailScenario) threadsOf(ctx context.Context, first, second string) (uuid.UUID, uuid.UUID, error) {
	one, err := s.threadOf(ctx, first)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	other, err := s.threadOf(ctx, second)
	return one, other, err
}

// shareAThread fails unless the mails titled first and second sit in one thread.
func (s *emailScenario) shareAThread(ctx context.Context, first, second string) error {
	one, other, err := s.threadsOf(ctx, first, second)
	if err != nil {
		return err
	}
	if one != other {
		return fmt.Errorf("the mails %q and %q sit in the threads %s and %s, want one thread", first, second, one, other)
	}
	return nil
}

// shareNoThread fails when the mails titled first and second sit in one thread.
func (s *emailScenario) shareNoThread(ctx context.Context, first, second string) error {
	one, other, err := s.threadsOf(ctx, first, second)
	if err != nil {
		return err
	}
	if one == other {
		return fmt.Errorf("the mails %q and %q both sit in the thread %s, want two threads", first, second, one)
	}
	return nil
}

// contactsShareNoThread fails when the contacts called first and second in the workspace hold mail of one thread.
func (s *emailScenario) contactsShareNoThread(ctx context.Context, first, second, workspace string) error {
	one, err := s.contactID(ctx, first, workspace)
	if err != nil {
		return err
	}
	other, err := s.contactID(ctx, second, workspace)
	if err != nil {
		return err
	}
	var shared bool
	if err := s.pool.QueryRow(ctx, contactsInOneThread, one, other).Scan(&shared); err != nil || shared {
		return fmt.Errorf("the contacts %s and %s share a thread %v (%v), want none", first, second, shared, err)
	}
	return nil
}

// threadLists fails unless the thread of the mail titled subject lists the subjects want in the order received.
func (s *emailScenario) threadLists(ctx context.Context, subject, want string) error {
	thread, err := s.threadOf(ctx, subject)
	if err != nil {
		return err
	}
	var listed string
	if err := s.pool.QueryRow(ctx, threadSubjects, thread).Scan(&listed); err != nil || listed != want {
		return fmt.Errorf("the thread of %q lists %q (%v), want %q", subject, listed, err, want)
	}
	return nil
}

// queuedData reads the data of every delivery queued for the event called name.
func (s *emailScenario) queuedData(ctx context.Context, name string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, queuedEvents, name)
	if err != nil {
		return nil, err
	}
	payloads, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	data := make([]map[string]any, 0, len(payloads))
	for _, payload := range payloads {
		var delivered struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal([]byte(payload), &delivered); err != nil {
			return nil, fmt.Errorf("reading the payload of %s: %w", name, err)
		}
		data = append(data, delivered.Data)
	}
	return data, nil
}

// onlyEvent reads the data of the one delivery queued for the event called name.
func (s *emailScenario) onlyEvent(ctx context.Context, name string) (map[string]any, error) {
	data, err := s.queuedData(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(data) != 1 {
		return nil, fmt.Errorf("%d deliveries are queued for %s, want 1", len(data), name)
	}
	return data[0], nil
}

// queuesNoEvent fails unless no delivery is queued for the event called name.
func (s *emailScenario) queuesNoEvent(ctx context.Context, name string) error {
	data, err := s.queuedData(ctx, name)
	if err != nil || len(data) != 0 {
		return fmt.Errorf("%d deliveries are queued for %s (%v), want none", len(data), name, err)
	}
	return nil
}

// linkedContactsOf reads the ids of the contacts the mail id is linked to, in the order of their text.
func (s *emailScenario) linkedContactsOf(ctx context.Context, id uuid.UUID) ([]any, error) {
	rows, err := s.pool.Query(ctx, linkedContacts, id)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	linked := make([]any, len(ids))
	for i, held := range ids {
		linked[i] = held
	}
	return linked, nil
}

// sortedList answers value sorted by its text when it is a list, value itself otherwise.
func sortedList(value any) any {
	list, isList := value.([]any)
	if !isList {
		return value
	}
	sorted := slices.Clone(list)
	slices.SortFunc(sorted, func(a, b any) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
	return sorted
}

// eventCarriesTheMail fails unless the one event called name carries the ids, contacts, subject and text of the mail.
func (s *emailScenario) eventCarriesTheMail(ctx context.Context, name, subject string) error {
	data, err := s.onlyEvent(ctx, name)
	if err != nil {
		return err
	}
	held, err := s.mail(ctx, subject)
	if err != nil {
		return err
	}
	contacts, err := s.linkedContactsOf(ctx, held.id)
	if err != nil {
		return err
	}
	want := map[string]any{
		"id": held.id.String(), "thread_id": held.thread.String(), "contact_ids": contacts,
		"subject": subject, "text": held.text,
	}
	got := maps.Clone(data)
	got["contact_ids"] = sortedList(data["contact_ids"])
	if !reflect.DeepEqual(got, want) || strings.Contains(held.text, "<") {
		return fmt.Errorf("the event %s carries %v, want %v with no markup", name, data, want)
	}
	return nil
}

// eventCarriesText fails unless the one event called name carries text as its text.
func (s *emailScenario) eventCarriesText(ctx context.Context, name, text string) error {
	data, err := s.onlyEvent(ctx, name)
	if err != nil {
		return err
	}
	if data["text"] != text {
		return fmt.Errorf("the event %s carries the text %q, want %q", name, data["text"], text)
	}
	return nil
}

// stop stops the server a scenario started and closes its pool.
func (s *emailScenario) stop() error {
	var err error
	if s.stopServing != nil {
		http.DefaultClient.CloseIdleConnections()
		s.stopServing()
		select {
		case err = <-s.served:
		case <-time.After(serveStopBudget):
			err = errors.New("AlphOne did not stop serving after the scenario")
		}
	}
	if s.pool != nil {
		s.pool.Close()
	}
	return err
}

func TestEmailIngest(t *testing.T) {
	runFeature(t, emailIngestFeature, initializeEmail(t))
}

func TestEmailThreads(t *testing.T) {
	runFeature(t, emailThreadsFeature, initializeEmail(t))
}

func TestEmailStorage(t *testing.T) {
	runFeature(t, emailStorageFeature, initializeEmail(t))
}

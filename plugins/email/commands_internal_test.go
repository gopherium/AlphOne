// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/framework/gonsole"

	"github.com/gopherium/alphone/sdk"
)

// errAnswerLost is the failure of a standard output that takes no answer.
var errAnswerLost = errors.New("standard output closed")

// lostAnswer is a standard output whose every write fails.
type lostAnswer struct{}

// Write returns errAnswerLost instead of taking any bytes.
func (lostAnswer) Write([]byte) (int, error) {
	return 0, errAnswerLost
}

// unreadableMail is a standard input whose every read fails.
type unreadableMail struct{}

// Read returns errEntropy instead of any bytes.
func (unreadableMail) Read([]byte) (int, error) {
	return 0, errEntropy
}

// commandLine is one line of email:ingest: its workspace, its flags, its standard input and whether it applies.
type commandLine struct {
	// workspace is the positional workspace argument.
	workspace string
	// args are the command's own flags as the line types them.
	args []string
	// stdin is the raw mail the line feeds.
	stdin io.Reader
	// apply reports a line that holds -yes.
	apply bool
	// stdout takes the answer, a buffer when nil.
	stdout io.Writer
}

// run reads the line through the flags email:ingest declares and runs the command, answering stdout and the error.
func (line commandLine) run(t *testing.T, p *ingester) (string, error) {
	t.Helper()
	command := p.ingestCommand()
	fs := flag.NewFlagSet(command.Name, flag.ContinueOnError)
	command.Flags(fs)
	if err := fs.Parse(line.args); err != nil {
		t.Fatalf("reading the flags %q: %v", line.args, err)
	}
	flags := map[string]string{}
	fs.Visit(func(f *flag.Flag) { flags[f.Name] = f.Value.String() })
	answered := &bytes.Buffer{}
	stdout := line.stdout
	if stdout == nil {
		stdout = answered
	}
	err := command.Run(t.Context(), sdk.Call{
		Args: []string{line.workspace}, Flags: flags, Stdin: line.stdin, Stdout: stdout, Stderr: io.Discard,
		Apply: line.apply,
	})
	return answered.String(), err
}

// ingestLine returns a line of email:ingest feeding raw to the workspace tenant, applying when apply is set.
func ingestLine(tenant uuid.UUID, raw []byte, apply bool, args ...string) commandLine {
	return commandLine{workspace: tenant.String(), args: args, stdin: bytes.NewReader(raw), apply: apply}
}

// fileRef returns the provider reference email:ingest gives the raw mail.
func fileRef(raw []byte) string {
	digest := sha256.Sum256(raw)
	return "file:" + hex.EncodeToString(digest[:])
}

// ccSalesMail returns a mail of Maria Perez to the mailbox with its alias in Cc.
func ccSalesMail() []byte {
	return rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Cc: sales@example.com",
		"Subject: Lamp colours", "Date: "+quoteDate, "Message-ID: <lamp-colours@example.com>",
		"Content-Type: text/plain", "", "Which colours does the lamp come in?")
}

// storedReceivedAt reads the received time of the one stored mail with subject.
func (it *ingestTest) storedReceivedAt(subject string) time.Time {
	it.t.Helper()
	var received time.Time
	if err := it.pool.QueryRow(it.t.Context(), "SELECT received_at FROM plugin_email.messages WHERE id = $1",
		it.mailTitled(subject)).Scan(&received); err != nil {
		it.t.Fatalf("reading the received time of %q: %v", subject, err)
	}
	return received.UTC()
}

func TestTheIngestCommandWritesAndNeedsTheMailboxCapability(t *testing.T) {
	t.Parallel()

	command := (&ingester{}).ingestCommand()

	if command.Name != "email:ingest" || !slices.Equal(command.Args, []string{"workspace"}) {
		t.Errorf("ingestCommand() = %s %v, want email:ingest <workspace>", command.Name, command.Args)
	}
	if !command.Writes || command.Capability != "email.manage_mailbox" || command.JSON || command.Summary == "" {
		t.Errorf("ingestCommand() writes %v under %q with JSON %v and summary %q, "+
			"want a write under email.manage_mailbox with no JSON and a summary",
			command.Writes, command.Capability, command.JSON, command.Summary)
	}
	fs := flag.NewFlagSet(command.Name, flag.ContinueOnError)
	command.Flags(fs)
	for _, name := range []string{"mailbox", "alias", "received", "sent-copy", "provider-thread"} {
		if fs.Lookup(name) == nil {
			t.Errorf("ingestCommand() declares no -%s, want it", name)
		}
	}
	if switched, ok := fs.Lookup("sent-copy").Value.(interface{ IsBoolFlag() bool }); !ok || !switched.IsBoolFlag() {
		t.Error("ingestCommand() declares -sent-copy as a value, want a switch")
	}
}

func TestFeedMailStoresTheMailOnStandardInputUnderYes(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	raw := lampMail()

	answered, err := ingestLine(sdk.DefaultTenantID, raw, true).run(t, it.ingester)

	if err != nil {
		t.Fatalf("feedMail() error = %v, want nil", err)
	}
	if answered != "stored\n" {
		t.Errorf("feedMail() answered %q, want the outcome alone, stored", answered)
	}
	id, err := readMessageIDByProviderRef(t.Context(), it.pool, sdk.DefaultTenantID, it.box.id, fileRef(raw))
	if err != nil || id != it.mailTitled("Your quote") {
		t.Errorf("the mail is stored under %v, %v, want it under file: and the SHA-256 of the raw mail", id, err)
	}
}

func TestFeedMailPreviewsWithoutYesAndWritesNothing(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	answered, err := ingestLine(sdk.DefaultTenantID, lampMail(), false).run(t, it.ingester)

	if err != nil {
		t.Fatalf("feedMail() error = %v, want nil", err)
	}
	if answered != "would answer stored\n" {
		t.Errorf("feedMail() answered %q, want the outcome the mail would reach", answered)
	}
	if rows := it.rows(sdk.DefaultTenantID); rows != 0 || len(it.store.savedTypes()) != 0 {
		t.Errorf("the preview left %d rows and saved %v, want nothing written", rows, it.store.savedTypes())
	}
}

func TestFeedMailPreviewsAFirstMailWithTheMailboxTheFlagsName(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	tenant := storeWorkspace(t, it.pool)
	it.knowContacts(tenant)

	answered, err := ingestLine(tenant, ccSalesMail(), false,
		"-mailbox", "info@example.com", "-alias", "sales@example.com").run(t, it.ingester)

	if err != nil {
		t.Fatalf("feedMail() error = %v, want nil", err)
	}
	if answered != "would answer stored\n" {
		t.Errorf("feedMail() answered %q, want the outcome the mail would reach", answered)
	}
	if got := it.matcher.calls(); !slices.EqualFunc(got, [][]string{{"maria.perez@example.com"}}, slices.Equal) {
		t.Errorf("the preview matched %v, want the mailbox and its alias left out as own addresses", got)
	}
	if boxes := mailboxRows(t, it.pool, tenant); boxes != 0 {
		t.Errorf("the preview stored %d mailboxes, want none", boxes)
	}
}

func TestFeedMailRefusesAWorkspaceWithNoMailboxWhenTheLineNamesNone(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	tenant := storeWorkspace(t, it.pool)
	it.knowContacts(tenant)

	for name, apply := range map[string]bool{"a preview": false, "a run under -yes": true} {
		_, err := ingestLine(tenant, lampMail(), apply, "-alias", "sales@example.com").run(t, it.ingester)

		if !errors.Is(err, errNoMailbox) || !errors.Is(err, gonsole.ErrMisused) {
			t.Errorf("%s: feedMail() error = %v, want a misused line naming -mailbox", name, err)
		}
	}
	if boxes := mailboxRows(t, it.pool, tenant); boxes != 0 || it.rows(tenant) != 0 {
		t.Errorf("the refused lines stored %d mailboxes and %d rows, want none", boxes, it.rows(tenant))
	}
}

func TestFeedMailStoresTheMailboxTheFlagsNameWhenTheWorkspaceHoldsNone(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	tenant := storeWorkspace(t, it.pool)
	it.knowContacts(tenant)

	answered, err := ingestLine(tenant, lampMail(), true,
		"-mailbox", " info@example.com ", "-alias", "sales@example.com, orders@example.com,,").run(t, it.ingester)

	if err != nil || answered != "stored\n" {
		t.Fatalf("feedMail() = %q, %v, want stored", answered, err)
	}
	box, held, err := readWorkspaceMailbox(t.Context(), it.pool, tenant)
	if err != nil || !held {
		t.Fatalf("readWorkspaceMailbox() = %v, %v, want the mailbox the flags name", held, err)
	}
	wantAliases := []string{"sales@example.com", "orders@example.com"}
	if box.address != "info@example.com" || !slices.Equal(box.aliases, wantAliases) {
		t.Errorf("the stored mailbox is %q with %q, want info@example.com with sales@ and orders@", box.address, box.aliases)
	}
	if _, err := readMessageIDByProviderRef(t.Context(), it.pool, tenant, box.id, fileRef(lampMail())); err != nil {
		t.Errorf("the mail is not in the new mailbox: %v", err)
	}
}

func TestFeedMailAsksTheGateBeforeItStoresAMailbox(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name     string
		closed   bool
		err      error
		answered string
	}{
		{name: "a closed workspace", closed: true, answered: "workspace_closed\n"},
		{name: "a gate that fails", err: errGateDown},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			it := newIngestTest(t)
			tenant := storeWorkspace(t, it.pool)
			it.knowContacts(tenant)
			it.gate.closed, it.gate.err = c.closed, c.err

			answered, err := ingestLine(tenant, lampMail(), true, "-mailbox", "info@example.com").run(t, it.ingester)

			if answered != c.answered || !errors.Is(err, c.err) {
				t.Errorf("feedMail() = %q, %v, want %q, %v", answered, err, c.answered, c.err)
			}
			if boxes := mailboxRows(t, it.pool, tenant); boxes != 0 || it.rows(tenant) != 0 {
				t.Errorf("the workspace holds %d mailboxes and %d rows, want none", boxes, it.rows(tenant))
			}
			if steps := it.log.steps(); !slices.Equal(steps, []string{"gate"}) {
				t.Errorf("the command called %q, want the gate alone", steps)
			}
		})
	}
}

func TestFeedMailKeepsTheMailboxAWorkspaceHolds(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	_, err := ingestLine(sdk.DefaultTenantID, lampMail(), true,
		"-mailbox", "other@example.com", "-alias", "more@example.com").run(t, it.ingester)

	if err != nil {
		t.Fatalf("feedMail() error = %v, want nil", err)
	}
	box, _, err := readWorkspaceMailbox(t.Context(), it.pool, sdk.DefaultTenantID)
	if err != nil || box.id != it.box.id || box.address != it.box.address {
		t.Errorf("the workspace holds %+v, %v, want its mailbox %+v left as it was", box, err, it.box)
	}
	if !slices.Equal(box.aliases, it.box.aliases) || mailboxRows(t, it.pool, sdk.DefaultTenantID) != 1 {
		t.Errorf("the workspace holds the aliases %v, want %v in its one mailbox", box.aliases, it.box.aliases)
	}
}

func TestFeedMailMarksACopyReadFromTheSentFolder(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	raw := infoMail()

	answered, err := ingestLine(sdk.DefaultTenantID, raw, true, "-sent-copy").run(t, it.ingester)

	if err != nil || answered != "stored\n" {
		t.Fatalf("feedMail() = %q, %v, want stored", answered, err)
	}
	if row := storedRow(t, it.pool, it.mailTitled("Your quote")); row.direction != outbound {
		t.Errorf("the Sent copy is stored %s, want outbound", row.direction)
	}
	var sentCopy bool
	if err := it.pool.QueryRow(t.Context(), "SELECT sent_copy FROM plugin_email.provider_refs WHERE provider_ref = $1",
		fileRef(raw)).Scan(&sentCopy); err != nil || !sentCopy {
		t.Errorf("the reference of the copy marks a Sent copy %v, %v, want true", sentCopy, err)
	}
}

func TestFeedMailHandsTheProviderThreadToTheThreading(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	for n := range 2 {
		if _, err := ingestLine(sdk.DefaultTenantID, numberedMail(n+1), true,
			"-provider-thread", "conversation-7").run(t, it.ingester); err != nil {
			t.Fatalf("feedMail() of order %d error = %v, want nil", n+1, err)
		}
	}

	first, second := storedRow(t, it.pool, it.mailTitled("Order 1")), storedRow(t, it.pool, it.mailTitled("Order 2"))
	if first.providerThreadID != "file:conversation-7" {
		t.Errorf("the mail keeps the provider thread %q, want file:conversation-7", first.providerThreadID)
	}
	if first.threadID != second.threadID {
		t.Error("two mails of one provider thread sit in two threads, want the provider thread to join them")
	}
}

func TestFeedMailAnswersDuplicateForTheSameFileTwice(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	if _, err := ingestLine(sdk.DefaultTenantID, lampMail(), true).run(t, it.ingester); err != nil {
		t.Fatalf("feedMail() error = %v, want nil", err)
	}

	answered, err := ingestLine(sdk.DefaultTenantID, lampMail(), true).run(t, it.ingester)

	if err != nil || answered != "duplicate\n" {
		t.Errorf("feedMail() of the same file again = %q, %v, want duplicate", answered, err)
	}
	if mails := it.mails(); mails != 1 {
		t.Errorf("the workspace holds %d mails, want 1", mails)
	}
}

func TestFeedMailTakesTheReceivedTimeFromTheLineOrElseTheClock(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	stated := time.Date(2026, time.November, 3, 10, 30, 0, 0, time.UTC)

	if _, err := ingestLine(sdk.DefaultTenantID, numberedMail(1), true,
		"-received", "2026-11-03T11:30:00+01:00").run(t, it.ingester); err != nil {
		t.Fatalf("feedMail() error = %v, want nil", err)
	}
	if _, err := ingestLine(sdk.DefaultTenantID, numberedMail(2), true).run(t, it.ingester); err != nil {
		t.Fatalf("feedMail() error = %v, want nil", err)
	}

	if got := it.storedReceivedAt("Order 1"); !got.Equal(stated) {
		t.Errorf("the mail fed with -received was received at %v, want %v", got, stated)
	}
	if got := it.storedReceivedAt("Order 2"); !got.Equal(ingestNow) {
		t.Errorf("the mail fed with no -received was received at %v, want the clock's %v", got, ingestNow)
	}
}

func TestFeedMailRefusesALineItCannotRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	lines := map[string]commandLine{
		"a workspace that is no uuid":        {workspace: "Acme", stdin: bytes.NewReader(lampMail()), apply: true},
		"a received time that does not read": ingestLine(sdk.DefaultTenantID, lampMail(), true, "-received", "yesterday"),
	}

	for name, line := range lines {
		_, err := line.run(t, it.ingester)

		if !errors.Is(err, gonsole.ErrMisused) {
			t.Errorf("%s: feedMail() error = %v, want a misused line", name, err)
		}
	}
	if rows := it.rows(sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the refused lines left %d rows, want none", rows)
	}
}

func TestFeedMailReportsAWorkspaceThatDoesNotExist(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	unknown := uuid.Must(uuid.NewV7())

	_, applied := ingestLine(unknown, lampMail(), true, "-mailbox", "info@example.com").run(t, it.ingester)
	it.gate.err = errGateDown
	_, previewed := ingestLine(unknown, lampMail(), false, "-mailbox", "info@example.com").run(t, it.ingester)

	if applied == nil || errors.Is(applied, gonsole.ErrMisused) {
		t.Errorf("feedMail() under -yes error = %v, want the refused mailbox of a workspace that does not exist", applied)
	}
	if !errors.Is(previewed, errGateDown) {
		t.Errorf("feedMail() of a preview error = %v, want the gate's refusal of the unknown workspace", previewed)
	}
}

func TestFeedMailReportsAnUnmigratedDatabase(t *testing.T) {
	t.Parallel()

	pool, _ := newUnmigratedPool(t)
	held, err := readSettings(sdk.Deps{})
	if err != nil {
		t.Fatalf("readSettings() error = %v, want nil", err)
	}
	p := newIngester(pool, sdk.Deps{}, held)

	for name, apply := range map[string]bool{"a preview": false, "a run under -yes": true} {
		_, err := ingestLine(sdk.DefaultTenantID, lampMail(), apply, "-mailbox", "info@example.com").run(t, p)

		if err == nil || errors.Is(err, gonsole.ErrMisused) {
			t.Errorf("%s: feedMail() error = %v, want the mailbox read the database refuses", name, err)
		}
	}
}

func TestFeedMailReportsAnIngestThatFails(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	it.gate.err = errGateDown

	_, err := ingestLine(sdk.DefaultTenantID, lampMail(), true).run(t, it.ingester)

	if !errors.Is(err, errGateDown) {
		t.Errorf("feedMail() error = %v, want the gate's failure", err)
	}
}

func TestFeedMailReportsAMailItCannotRead(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)
	line := commandLine{workspace: sdk.DefaultTenantID.String(), stdin: unreadableMail{}, apply: true}

	_, err := line.run(t, it.ingester)

	if !errors.Is(err, errEntropy) {
		t.Errorf("feedMail() error = %v, want the failed read of standard input", err)
	}
}

func TestFeedMailReportsAnAnswerItCannotWrite(t *testing.T) {
	t.Parallel()

	it := newIngestTest(t)

	for name, apply := range map[string]bool{"a preview": false, "a run under -yes": true} {
		line := ingestLine(sdk.DefaultTenantID, lampMail(), apply)
		line.stdout = lostAnswer{}

		_, err := line.run(t, it.ingester)

		if !errors.Is(err, errAnswerLost) {
			t.Errorf("%s: feedMail() error = %v, want the failed write of the answer", name, err)
		}
	}
}

func TestAliasesOfReadsACommaList(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"":                  nil,
		" , ":               nil,
		"sales@example.com": {"sales@example.com"},
		" sales@example.com ,orders@example.com,": {"sales@example.com", "orders@example.com"},
	}

	for text, want := range tests {
		if got := aliasesOf(text); !slices.Equal(got, want) {
			t.Errorf("aliasesOf(%q) = %q, want %q", text, got, want)
		}
	}
}

// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// ingestCommandName is the command that feeds one raw mail into the mailbox of a workspace.
const ingestCommandName = "email:ingest"

// manageMailbox is the capability an account needs to feed mail into a mailbox.
const manageMailbox = "email.manage_mailbox"

const (
	// mailboxFlag names the address of the mailbox stored for a workspace that holds none.
	mailboxFlag = "mailbox"
	// aliasFlag names the other addresses that reach that new mailbox, a comma list.
	aliasFlag = "alias"
	// receivedFlag names the time the mailbox received the mail.
	receivedFlag = "received"
	// sentCopyFlag marks a copy read from the Sent folder.
	sentCopyFlag = "sent-copy"
	// providerThreadFlag names the provider's own thread id of the mail.
	providerThreadFlag = "provider-thread"
)

// filePrefix starts every provider reference and provider thread id of the mail email:ingest reads.
const filePrefix = "file:"

// errNoMailbox reports a workspace that holds no mailbox on a line that names none.
var errNoMailbox = errors.New("email:ingest: the workspace holds no mailbox, name its address with -mailbox")

// targetMailbox is the mailbox email:ingest feeds a mail into, beside its workspace.
type targetMailbox struct {
	// tenant names the workspace the mailbox serves.
	tenant uuid.UUID
	// box is the workspace's mailbox, or the one the flags name when it holds none.
	box mailbox
	// held reports a mailbox the workspace already holds.
	held bool
}

// ingestCommand returns email:ingest, which feeds the raw mail on standard input into a workspace's mailbox.
func (p *ingester) ingestCommand() sdk.Command {
	return sdk.Command{
		Name:       ingestCommandName,
		Summary:    "feed one raw mail on standard input into the mailbox of a workspace",
		Args:       []string{"workspace"},
		Flags:      declareIngestFlags,
		Writes:     true,
		Capability: manageMailbox,
		Run:        p.feedMail,
	}
}

// declareIngestFlags declares the flags of email:ingest on fs.
func declareIngestFlags(fs *flag.FlagSet) {
	fs.String(mailboxFlag, "", "`email` address of the mailbox to store when the workspace holds none")
	fs.String(aliasFlag, "", "comma separated `emails` that also reach that new mailbox")
	fs.String(receivedFlag, "", "`time` the mailbox received the mail, such as 2026-11-02T09:00:00Z, now when unset")
	fs.Bool(sentCopyFlag, false, "mark the mail as a copy read from the Sent folder")
	fs.String(providerThreadFlag, "", "the provider's own thread `id` of the mail")
}

// feedMail feeds the raw mail on standard input into the mailbox of the workspace the call names, a preview until -yes.
func (p *ingester) feedMail(ctx context.Context, call sdk.Call) error {
	tenant, err := uuid.Parse(call.Args[0])
	if err != nil {
		return sdk.Misuse(fmt.Errorf("%s: the workspace %q is no uuid", ingestCommandName, call.Args[0]))
	}
	in, err := readCommandMail(call, p.now)
	if err != nil {
		return err
	}
	target, err := p.readTargetMailbox(ctx, tenant, call.Flags)
	if err != nil {
		return err
	}
	answerMail := p.previewCommandMail
	if call.Apply {
		answerMail = p.storeCommandMail
	}
	answer, err := answerMail(ctx, target, in)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(call.Stdout, answer)
	return err
}

// readCommandMail reads the raw mail on the standard input of call and the flags that describe it.
func readCommandMail(call sdk.Call, now func() time.Time) (Incoming, error) {
	received, err := parseReceivedTime(call.Flags[receivedFlag], now)
	if err != nil {
		return Incoming{}, err
	}
	raw, err := io.ReadAll(call.Stdin)
	if err != nil {
		return Incoming{}, fmt.Errorf("email: read the raw mail: %w", err)
	}
	digest := sha256.Sum256(raw)
	in := Incoming{
		Raw: bytes.NewReader(raw), ProviderRef: filePrefix + hex.EncodeToString(digest[:]), ReceivedAt: received,
		SentCopy: call.Flags[sentCopyFlag] == "true",
	}
	if thread := call.Flags[providerThreadFlag]; thread != "" {
		in.ProviderThreadID = filePrefix + thread
	}
	return in, nil
}

// parseReceivedTime returns the RFC 3339 time text names, the time now answers when text is empty.
func parseReceivedTime(text string, now func() time.Time) (time.Time, error) {
	if text == "" {
		return now(), nil
	}
	received, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, sdk.Misuse(fmt.Errorf("%s: -%s %q is not a time such as 2026-11-02T09:00:00Z",
			ingestCommandName, receivedFlag, text))
	}
	return received, nil
}

// readTargetMailbox returns the mailbox of the workspace tenant, else the one the flags name, and whether it is stored.
func (p *ingester) readTargetMailbox(
	ctx context.Context, tenant uuid.UUID, flags map[string]string,
) (targetMailbox, error) {
	box, held, err := readWorkspaceMailbox(ctx, p.pool, tenant)
	if err != nil || held {
		return targetMailbox{tenant: tenant, box: box, held: held}, err
	}
	address := strings.TrimSpace(flags[mailboxFlag])
	if address == "" {
		return targetMailbox{}, sdk.Misuse(errNoMailbox)
	}
	return targetMailbox{tenant: tenant, box: mailbox{address: address, aliases: aliasesOf(flags[aliasFlag])}}, nil
}

// aliasesOf returns every address of the comma list text, trimmed, leaving out the empty ones.
func aliasesOf(text string) []string {
	var aliases []string
	for alias := range strings.SplitSeq(text, ",") {
		if trimmed := strings.TrimSpace(alias); trimmed != "" {
			aliases = append(aliases, trimmed)
		}
	}
	return aliases
}

// previewCommandMail returns the answer naming the outcome in would reach through target, writing nothing.
func (p *ingester) previewCommandMail(ctx context.Context, target targetMailbox, in Incoming) (string, error) {
	outcome, err := p.preview(ctx, target.tenant, target.box, in)
	if err != nil {
		return "", err
	}
	return "would answer " + string(outcome), nil
}

// storeCommandMail stores in through target and returns its outcome, storing the mailbox first for an open workspace.
func (p *ingester) storeCommandMail(ctx context.Context, target targetMailbox, in Incoming) (string, error) {
	if !target.held {
		accepted, err := p.askGate(sdk.WithTenant(ctx, target.tenant), target.tenant)
		if err != nil {
			return "", err
		}
		if !accepted {
			return string(outcomeWorkspaceClosed), nil
		}
		target.box, err = ensureMailbox(ctx, p.pool, target.tenant, target.box.address, target.box.aliases)
		if err != nil {
			return "", err
		}
	}
	in.MailboxID = target.box.id
	outcome, err := p.ingest(ctx, target.tenant, in)
	return string(outcome), err
}

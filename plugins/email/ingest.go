// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

// emailChannel is the channel the addresses of a mail match contacts on.
const emailChannel sdk.Channel = "email"

// Outcome names what one ingest did with a mail.
type Outcome string

const (
	// outcomeStored marks a mail stored as a new row.
	outcomeStored Outcome = "stored"
	// outcomeStoredHeadersOnly marks a mail stored as a new row with its headers alone.
	outcomeStoredHeadersOnly Outcome = "stored_headers_only"
	// outcomeLinked marks a mail already stored that gained a link to a contact.
	outcomeLinked Outcome = "linked"
	// outcomeDuplicate marks a mail already stored that gained no link.
	outcomeDuplicate Outcome = "duplicate"
	// outcomeNoKnownContact marks a mail that names no known contact and left nothing behind.
	outcomeNoKnownContact Outcome = "no_known_contact"
	// outcomeHeadersOnlyNoKnownContact marks a mail read by its headers alone that names no known contact.
	outcomeHeadersOnlyNoKnownContact Outcome = "headers_only_no_known_contact"
	// outcomeFailed marks a mail set aside and never tried again.
	outcomeFailed Outcome = "failed"
	// outcomeWorkspaceClosed marks a mail for a workspace that no longer takes what a channel delivers.
	outcomeWorkspaceClosed Outcome = "workspace_closed"
)

const (
	// unreadableMalformed is why a mail too broken to parse was read by its headers alone.
	unreadableMalformed = "malformed"
	// unreadableOverALimit is why a mail over a parse limit was read by its headers alone.
	unreadableOverALimit = "over_a_limit"
)

// errNoReceivedTime reports a mail handed over with no time its mailbox received it.
var errNoReceivedTime = errors.New("email: a mail needs the time its mailbox received it")

// Incoming is one raw mail on its way into a mailbox.
type Incoming struct {
	// MailboxID names the mailbox the mail reached.
	MailboxID uuid.UUID
	// Raw reads the raw mail.
	Raw io.Reader
	// ProviderRef is the provider's own id of this copy, behind a prefix the caller picks.
	ProviderRef string
	// ProviderThreadID is the provider's own thread id, behind a prefix the caller picks, empty for none.
	ProviderThreadID string
	// ReceivedAt is when the mailbox received the mail.
	ReceivedAt time.Time
	// SentCopy marks a copy read from the mailbox's Sent folder.
	SentCopy bool
	// Backfill marks old mail a first import or a backfill brings in.
	Backfill bool
	// read is the raw mail its caller already read within the message cap, nil when Raw still holds it.
	read *rawRead
}

// ingester stores raw mail on the known contacts it names.
type ingester struct {
	// pool reaches the mail of every workspace.
	pool *pgxpool.Pool
	// gate answers whether a workspace still takes what a channel delivers.
	gate sdk.TenantGate
	// matcher finds the contacts that own the addresses of a mail.
	matcher sdk.ContactMatcher
	// matchMax is the most addresses one call of matcher takes.
	matchMax int
	// files keeps the files of each mail through the queue.
	files mailFiles
	// publisher announces each new mail from a person.
	publisher sdk.Publisher
	// logger records by ids alone what a mail could not keep.
	logger *slog.Logger
	// parse bounds what Parse and ReadHeaders read of one raw mail.
	parse message.Limits
	// caps bound how much of each mail is kept and published.
	caps mailCaps
	// times bound how long a subject joins a thread, a mail keeps failing and the record of a failure runs.
	times timeLimits
	// slots holds one token for each mail in memory, its capacity the most mails a process holds at once.
	slots chan struct{}
	// now answers the time a failure is judged and recorded at.
	now func() time.Time
}

// rawRead is one raw mail read within the message cap, beside the size and the hash of the whole mail.
type rawRead struct {
	// bytes holds the mail up to one byte past the message cap.
	bytes []byte
	// size is the size of the whole mail.
	size int64
	// sha256 is the hash of the whole mail.
	sha256 []byte
}

// parsedMail is one raw mail read, classified and matched, on its way to its row.
type parsedMail struct {
	// in is the mail as its caller handed it.
	in Incoming
	// box is the mailbox the mail reached.
	box mailbox
	// raw is the raw mail as read.
	raw rawRead
	// message is the mail as Parse or ReadHeaders read it.
	message *message.Message
	// unreadable is why only the headers of the mail were read, nil when it read whole.
	unreadable error
	// automation names the kind of program that sent the mail, empty for a person.
	automation message.Automation
	// marks are what the parts of the mail mark it as.
	marks message.Marks
	// participants are the addresses the mail names beside their fields, cut to the recipient cap.
	participants []participant
	// contacts maps each email the mail names to the contact it matched.
	contacts map[string]uuid.UUID
	// dedupeKey names the mail inside its mailbox.
	dedupeKey []byte
}

// committedMail is what the transaction of one mail wrote.
type committedMail struct {
	// outcome is what the transaction did with the mail.
	outcome Outcome
	// id names the new row, uuid.Nil for a copy of a stored mail.
	id uuid.UUID
	// row is the new row with its thread.
	row messageRow
	// idHolder names the stored mail holding the key of the mail's Message-ID, uuid.Nil when the mail names none.
	idHolder uuid.UUID
}

// newIngester returns the ingester of mail over pool within s, the gate and the two stores left to the host.
func newIngester(pool *pgxpool.Pool, deps sdk.Deps, s settings) *ingester {
	return &ingester{
		pool: pool, matcher: deps.Matcher, matchMax: deps.ContactMatchMax, publisher: deps.Events,
		logger: deps.Logger, parse: s.parse, caps: s.caps, times: s.times,
		files: mailFiles{pool: pool, logger: deps.Logger, caps: s.files, cleanupTimeout: s.times.cleanupTimeout},
		slots: make(chan struct{}, s.caps.mailsInMemoryMax), now: time.Now,
	}
}

// ingest stores in on the known contacts of the workspace tenant and announces a new mail from a person.
func (p *ingester) ingest(ctx context.Context, tenant uuid.UUID, in Incoming) (Outcome, error) {
	outcome, arrival, err := p.store(ctx, tenant, in)
	if arrival.id != uuid.Nil {
		publishArrival(ctx, p.publisher, tenant, arrival, p.caps.eventTextMaxBytes)
	}
	return outcome, err
}

// store keeps in on the known contacts of the workspace tenant, answering the mail a new row announces.
func (p *ingester) store(ctx context.Context, tenant uuid.UUID, in Incoming) (Outcome, announcedMail, error) {
	if in.ReceivedAt.IsZero() {
		return "", announcedMail{}, errNoReceivedTime
	}
	ctx = sdk.WithTenant(ctx, tenant)
	accepted, err := p.askGate(ctx, tenant)
	if err != nil {
		return "", announcedMail{}, err
	}
	if !accepted {
		return outcomeWorkspaceClosed, announcedMail{}, nil
	}
	box, err := readMailbox(ctx, p.pool, tenant, in.MailboxID)
	if err != nil {
		return "", announcedMail{}, err
	}
	outcome, ended, err := p.readReferenceOutcome(ctx, tenant, box.id, in.ProviderRef)
	if outcome == outcomeDuplicate {
		return p.deleteStaleFailure(ctx, tenant, box.id, in.ProviderRef)
	}
	if ended {
		return outcome, announcedMail{}, err
	}
	return p.readInSlot(ctx, tenant, box, in, p.keepMail)
}

// deleteStaleFailure deletes the failure a stored mail left at the reference ref of the mailbox, answering duplicate.
func (p *ingester) deleteStaleFailure(
	ctx context.Context, tenant, mailboxID uuid.UUID, ref string,
) (Outcome, announcedMail, error) {
	if err := deleteFailedMail(ctx, p.pool, tenant, mailboxID, ref); err != nil {
		return "", announcedMail{}, err
	}
	return outcomeDuplicate, announcedMail{}, nil
}

// preview answers the outcome in would reach in the mailbox box of the workspace tenant, writing nothing.
func (p *ingester) preview(ctx context.Context, tenant uuid.UUID, box mailbox, in Incoming) (Outcome, error) {
	ctx = sdk.WithTenant(ctx, tenant)
	accepted, err := p.askGate(ctx, tenant)
	if err != nil {
		return "", err
	}
	if !accepted {
		return outcomeWorkspaceClosed, nil
	}
	outcome, ended, err := p.readReferenceOutcome(ctx, tenant, box.id, in.ProviderRef)
	if ended {
		return outcome, err
	}
	outcome, _, err = p.readInSlot(ctx, tenant, box, in, p.previewMail)
	return outcome, err
}

// askGate reports whether the workspace tenant still takes what a channel delivers.
func (p *ingester) askGate(ctx context.Context, tenant uuid.UUID) (bool, error) {
	accepted, err := p.gate.AcceptsMachineTraffic(ctx, tenant)
	if err != nil {
		return false, fmt.Errorf("email: ask whether the workspace takes mail: %w", err)
	}
	return accepted, nil
}

// readInSlot hands in, read inside an ingest slot, to finish.
func (p *ingester) readInSlot(
	ctx context.Context, tenant uuid.UUID, box mailbox, in Incoming,
	finish func(context.Context, uuid.UUID, parsedMail) (Outcome, announcedMail, error),
) (Outcome, announcedMail, error) {
	release, err := p.takeSlot(ctx)
	if err != nil {
		return "", announcedMail{}, err
	}
	defer release()
	mail, err := p.readIncoming(ctx, box, in)
	if err != nil {
		return "", announcedMail{}, err
	}
	return finish(ctx, tenant, mail)
}

// readReferenceOutcome answers duplicate or failed when the reference ends the ingest, and whether it ends it.
func (p *ingester) readReferenceOutcome(
	ctx context.Context, tenant, mailboxID uuid.UUID, ref string,
) (Outcome, bool, error) {
	_, err := readMessageIDByProviderRef(ctx, p.pool, tenant, mailboxID, ref)
	if err == nil {
		return outcomeDuplicate, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", true, err
	}
	failure, failed, err := readFailedMail(ctx, p.pool, tenant, mailboxID, ref)
	if err != nil {
		return "", true, err
	}
	if failed && failure.isSetAside(p.now(), p.times.retryWindow) {
		return outcomeFailed, true, nil
	}
	return "", false, nil
}

// takeSlot waits for a free ingest slot until ctx ends, answering the call that frees it.
func (p *ingester) takeSlot(ctx context.Context) (func(), error) {
	select {
	case p.slots <- struct{}{}:
		return func() { <-p.slots }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("email: wait for an ingest slot: %w", ctx.Err())
	}
}

// readIncoming reads in within the parse limits, classifies it, matches its addresses and computes its dedupe key.
func (p *ingester) readIncoming(ctx context.Context, box mailbox, in Incoming) (parsedMail, error) {
	raw, err := p.readRawOf(in)
	if err != nil {
		return parsedMail{}, err
	}
	m, unreadable := readMessage(raw.bytes, p.parse)
	mail := parsedMail{
		in: in, box: box, raw: raw, message: m, unreadable: unreadable, automation: message.Classify(m).Automation,
		marks: message.Mark(m), participants: p.participantsOf(m, in.SentCopy),
	}
	mail.dedupeKey = dedupeKey(mail)
	mail.contacts, err = p.matchContacts(ctx, mail.participants, box)
	return mail, err
}

// readRawOf returns the raw mail the caller of in already read, else reads in.Raw within the message cap.
func (p *ingester) readRawOf(in Incoming) (rawRead, error) {
	if in.read != nil {
		return *in.read, nil
	}
	return readRaw(in.Raw, p.parse.MaxBytes)
}

// readRaw reads r whole, keeping at most limit and one more byte in memory and counting and hashing every byte.
func readRaw(r io.Reader, limit int64) (rawRead, error) {
	hash := sha256.New()
	tee := io.TeeReader(r, hash)
	kept, err := io.ReadAll(io.LimitReader(tee, limit+1))
	if err != nil {
		return rawRead{}, fmt.Errorf("email: read the raw mail: %w", err)
	}
	rest, err := io.Copy(io.Discard, tee)
	if err != nil {
		return rawRead{}, fmt.Errorf("email: read the raw mail past the message cap: %w", err)
	}
	return rawRead{bytes: kept, size: int64(len(kept)) + rest, sha256: hash.Sum(nil)}, nil
}

// readMessage reads raw within limits, answering its headers alone and why when it is over a limit or too broken.
func readMessage(raw []byte, limits message.Limits) (m *message.Message, unreadable error) {
	m, err := message.ParseBytes(raw, limits)
	if err != nil {
		headers, _ := message.ReadHeaders(raw, limits)
		return headers, err
	}
	return m, nil
}

// participantsOf returns the addresses of m beside their fields, Bcc on a Sent copy alone, names cut, up to the cap.
func (p *ingester) participantsOf(m *message.Message, sentCopy bool) []participant {
	var bcc []message.Address
	if sentCopy {
		bcc = m.Bcc
	}
	listed := slices.Concat(p.withRole("from", []message.Address{m.From}), p.withRole("reply_to", m.ReplyTo),
		p.withRole("to", m.To), p.withRole("cc", m.Cc), p.withRole("bcc", bcc))
	listed = slices.DeleteFunc(listed, func(entry participant) bool { return entry.Email == "" })
	return listed[:min(len(listed), p.caps.recipientsMax)]
}

// withRole returns each of addresses as a participant in role, its name cut to the name cap.
func (p *ingester) withRole(role string, addresses []message.Address) []participant {
	listed := make([]participant, len(addresses))
	for i, address := range addresses {
		listed[i] = participant{Role: role, Name: cutOnLetter(address.Name, p.caps.nameMaxBytes), Email: address.Email}
	}
	return listed
}

// matchContacts returns the contact each email of participants names, the own addresses of the mailbox left out.
func (p *ingester) matchContacts(
	ctx context.Context, participants []participant, box mailbox,
) (map[string]uuid.UUID, error) {
	addresses := make([]message.Address, len(participants))
	for i, entry := range participants {
		addresses[i] = message.Address{Name: entry.Name, Email: entry.Email}
	}
	contacts := map[string]uuid.UUID{}
	for batch := range slices.Chunk(message.WithoutOwn(addresses, box.ownAddresses()), p.matchMax) {
		emails := make([]string, len(batch))
		for i, address := range batch {
			emails[i] = address.Email
		}
		matched, err := p.matcher.MatchIdentities(ctx, emailChannel, emails)
		if err != nil {
			return nil, fmt.Errorf("email: match the contacts of a mail: %w", err)
		}
		for email, contact := range matched {
			contacts[email] = contact.ID
		}
	}
	return contacts, nil
}

// keepMail stores mail as a new row, or links it to the stored mail it copies, unless it names no known contact.
func (p *ingester) keepMail(ctx context.Context, tenant uuid.UUID, mail parsedMail) (Outcome, announcedMail, error) {
	if len(mail.contacts) == 0 {
		p.logUnknown(ctx, tenant, mail)
		return unknownOutcome(mail), announcedMail{}, nil
	}
	stored, err := p.readStoredID(ctx, tenant, mail)
	if err != nil {
		return "", announcedMail{}, err
	}
	if stored == uuid.Nil {
		return p.storeNew(ctx, tenant, mail)
	}
	outcome, err := p.commitCopy(ctx, tenant, mail)
	if err != nil {
		outcome, err = p.setAside(ctx, tenant, mail, nil, err)
	}
	return outcome, announcedMail{}, err
}

// previewMail answers the outcome keeping mail would reach, writing nothing.
func (p *ingester) previewMail(ctx context.Context, tenant uuid.UUID, mail parsedMail) (Outcome, announcedMail, error) {
	if len(mail.contacts) == 0 {
		return unknownOutcome(mail), announcedMail{}, nil
	}
	stored, err := p.readStoredID(ctx, tenant, mail)
	switch {
	case err != nil:
		return "", announcedMail{}, err
	case stored == uuid.Nil:
		return newRowOutcome(mail), announcedMail{}, nil
	}
	newLinks, err := countNewLinks(ctx, p.pool, tenant, stored, sortedContacts(mail.contacts))
	if err != nil {
		return "", announcedMail{}, err
	}
	return copyOutcome(newLinks), announcedMail{}, nil
}

// readStoredID returns the stored mail the dedupe key of mail names in its mailbox, uuid.Nil when none.
func (p *ingester) readStoredID(ctx context.Context, tenant uuid.UUID, mail parsedMail) (uuid.UUID, error) {
	id, err := readMessageIDByDedupeKey(ctx, p.pool, tenant, mail.box.id, mail.dedupeKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	return id, err
}

// unknownOutcome returns the outcome of mail when it names no known contact.
func unknownOutcome(mail parsedMail) Outcome {
	if mail.unreadable != nil {
		return outcomeHeadersOnlyNoKnownContact
	}
	return outcomeNoKnownContact
}

// newRowOutcome returns the outcome of mail when it is stored as a new row.
func newRowOutcome(mail parsedMail) Outcome {
	if mail.unreadable != nil {
		return outcomeStoredHeadersOnly
	}
	return outcomeStored
}

// copyOutcome returns the outcome of a copy of a stored mail that adds newLinks links.
func copyOutcome(newLinks int64) Outcome {
	if newLinks > 0 {
		return outcomeLinked
	}
	return outcomeDuplicate
}

// logUnknown logs the mailbox and the reason of a mail read by its headers alone that names no known contact.
func (p *ingester) logUnknown(ctx context.Context, tenant uuid.UUID, mail parsedMail) {
	if mail.unreadable != nil {
		p.logger.InfoContext(ctx, "email: a mail read by its headers alone names no known contact",
			"workspace", tenant, "mailbox", mail.box.id, "reason", unreadableReason(mail.unreadable))
	}
}

// unreadableReason returns why a mail was read by its headers alone as a fixed word, never the text of err.
func unreadableReason(err error) string {
	if errors.Is(err, message.ErrMalformed) {
		return unreadableMalformed
	}
	return unreadableOverALimit
}

// storeNew stores mail as a new row with its files, setting it aside when a step from its files on fails.
func (p *ingester) storeNew(ctx context.Context, tenant uuid.UUID, mail parsedMail) (Outcome, announcedMail, error) {
	row := p.rowOf(mail)
	files := p.files.filesOf(mail.message, mail.raw.size <= p.parse.MaxBytes)
	state, err := p.files.checkWorkspaceLimit(ctx, tenant, storedSize(row, files))
	if err != nil {
		return "", announcedMail{}, err
	}
	kept, err := p.files.keepFiles(ctx, tenant, files, state)
	if err != nil {
		outcome, failure := p.setAside(ctx, tenant, mail, nil, err)
		return outcome, announcedMail{}, failure
	}
	written, err := p.commitMail(ctx, tenant, mail, withFiles(row, kept), kept)
	if err != nil {
		outcome, failure := p.setAside(ctx, tenant, mail, kept, err)
		return outcome, announcedMail{}, failure
	}
	p.logStored(ctx, tenant, written)
	return written.outcome, announcedMail{
		id: written.id, row: written.row, contacts: sortedContacts(mail.contacts), from: mail.message.From,
		own: mail.box.ownAddresses(), backfill: mail.in.Backfill,
	}, nil
}

// rowOf returns the row of mail as it is before its thread and its files are known.
func (p *ingester) rowOf(mail parsedMail) messageRow {
	m := mail.message
	row := messageRow{
		mailboxID: mail.box.id, providerThreadID: mail.in.ProviderThreadID, dedupeKey: mail.dedupeKey,
		internetMessageID: m.MessageID, inReplyToIDs: capIDs(m.InReplyTo, p.caps.threadReferencesMax),
		referenceIDs: capIDs(m.References, p.caps.threadReferencesMax), direction: inbound,
		subject: cutOnLetter(m.Subject, p.caps.subjectMaxBytes), participants: mail.participants, sentAt: m.Date,
		receivedAt: mail.in.ReceivedAt, bodyText: m.Text, safeHTML: message.Sanitize(m.HTML, nil),
		bodyState: bodyStateOf(mail), skippedParts: m.SkippedParts, automation: mail.automation,
		encrypted: mail.marks.Encrypted, signed: mail.marks.Signed, winmail: mail.marks.Winmail,
		inviteMethod: mail.marks.InviteMethod, rawSize: mail.raw.size, rawSHA256: mail.raw.sha256,
	}
	if mail.in.SentCopy {
		row.direction = outbound
	}
	if !mail.marks.Encrypted {
		row.snippet, _ = message.Snippet(m.Text, p.caps.snippetLetters)
	}
	return row
}

// bodyStateOf returns whether the text of mail is kept whole, cut at the body cap, or missing behind its headers.
func bodyStateOf(mail parsedMail) string {
	switch {
	case mail.unreadable != nil:
		return bodyHeadersOnly
	case mail.message.BodyCut:
		return bodyCut
	}
	return bodyWhole
}

// textSize returns the bytes of text row keeps: its body, its safe HTML, its snippet and its participants.
func textSize(row messageRow) int64 {
	return int64(len(row.bodyText)+len(row.safeHTML)+len(row.snippet)) + participantsSize(row.participants)
}

// storedSize returns the bytes a mail of row and files adds to its workspace if the store keeps each file it is sent.
func storedSize(row messageRow, files []mailFile) int64 {
	size := textSize(row)
	for _, file := range files {
		if !file.tooLarge {
			size += int64(len(file.content))
		}
	}
	return size
}

// withFiles returns row holding the raw copy the first of kept describes and the bytes the mail adds to its workspace.
func withFiles(row messageRow, kept []keptFile) messageRow {
	row.rawFileID, row.rawState = kept[0].fileID(), kept[0].state
	row.storedBytes = textSize(row)
	for _, file := range kept {
		if file.fileID() != uuid.Nil {
			row.storedBytes += file.size
		}
	}
	return row
}

// attachmentRows returns a row for each attachment and inline image of m beside its kept file, names and ids cut.
func attachmentRows(m *message.Message, kept []keptFile, nameMaxBytes int) []attachmentRow {
	parts := slices.Concat(m.Attachments, m.InlineImages)
	rows := make([]attachmentRow, len(parts))
	for i, part := range parts {
		file := kept[i+1]
		rows[i] = attachmentRow{
			position: i, filename: cutOnLetter(part.Name, nameMaxBytes), contentType: file.contentType, size: file.size,
			sha256: file.sha256, inline: i >= len(m.Attachments), contentID: cutOnLetter(part.ContentID, nameMaxBytes),
			fileID: file.fileID(), state: file.state,
		}
	}
	return rows
}

// threadMailOf returns what choosing the thread of mail reads of it, its subject as its row keeps it.
func (p *ingester) threadMailOf(mail parsedMail, subject string) threadMail {
	return threadMail{
		mailboxID: mail.box.id, message: mail.message, subject: subject, receivedAt: mail.in.ReceivedAt,
		sentCopy: mail.in.SentCopy, automated: mail.automation != message.NotAutomated, contacts: mail.contacts,
		keys: p.threadKeysOf(mail),
	}
}

// threadKeysOf returns the thread keys of mail, its reply ids cut to the thread references cap.
func (p *ingester) threadKeysOf(mail parsedMail) mailKeys {
	return keysOf(mail.message, mail.in.ProviderThreadID, p.caps.threadReferencesMax)
}

// sortedContacts returns each contact of a mail once, in the order of their ids.
func sortedContacts(contacts map[string]uuid.UUID) []uuid.UUID {
	return slices.Compact(slices.SortedFunc(maps.Values(contacts), func(a, b uuid.UUID) int {
		return bytes.Compare(a[:], b[:])
	}))
}

// referenceOf returns the provider reference of mail naming the stored mail id.
func referenceOf(mail parsedMail, id uuid.UUID) providerRef {
	return providerRef{mailboxID: mail.box.id, ref: mail.in.ProviderRef, messageID: id, sentCopy: mail.in.SentCopy}
}

// commitMail stores mail with its kept files in one transaction, or links it to the stored mail it copies.
func (p *ingester) commitMail(
	ctx context.Context, tenant uuid.UUID, mail parsedMail, row messageRow, kept []keptFile,
) (committedMail, error) {
	var written committedMail
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		written, err = p.writeMail(ctx, tx, tenant, mail, row, kept)
		return err
	})
	return written, err
}

// writeMail stores mail as a new row inside tx under the mailbox lock, or links it to the stored mail it copies.
func (p *ingester) writeMail(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail parsedMail, row messageRow, kept []keptFile,
) (committedMail, error) {
	if err := lockMailbox(ctx, tx, tenant, mail.box.id); err != nil {
		return committedMail{}, err
	}
	stored, err := findStoredMail(ctx, tx, tenant, mail)
	switch {
	case err == nil:
		outcome, err := p.linkCopy(ctx, tx, tenant, mail, stored, kept)
		return committedMail{outcome: outcome}, err
	case !errors.Is(err, pgx.ErrNoRows):
		return committedMail{}, err
	}
	return p.insertMail(ctx, tx, tenant, mail, row, kept)
}

// commitCopy links mail, which its dedupe key found stored, to that stored mail in one transaction.
func (p *ingester) commitCopy(ctx context.Context, tenant uuid.UUID, mail parsedMail) (Outcome, error) {
	var outcome Outcome
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		outcome, err = p.writeCopy(ctx, tx, tenant, mail)
		return err
	})
	return outcome, err
}

// writeCopy links mail to the stored mail it copies inside tx under the mailbox lock, answering a miss as it reads.
func (p *ingester) writeCopy(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail parsedMail) (Outcome, error) {
	if err := lockMailbox(ctx, tx, tenant, mail.box.id); err != nil {
		return "", err
	}
	stored, err := findStoredMail(ctx, tx, tenant, mail)
	if err != nil {
		return "", err
	}
	return p.linkCopy(ctx, tx, tenant, mail, stored, nil)
}

// findStoredMail returns the stored mail the reference or the dedupe key of mail names, wrapping pgx.ErrNoRows if none.
func findStoredMail(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail parsedMail) (uuid.UUID, error) {
	id, err := readMessageIDByProviderRef(ctx, tx, tenant, mail.box.id, mail.in.ProviderRef)
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	return readMessageIDByDedupeKey(ctx, tx, tenant, mail.box.id, mail.dedupeKey)
}

// insertMail stores mail as a new row of the thread it joins inside tx, with every part, its usage counted last.
func (p *ingester) insertMail(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail parsedMail, row messageRow, kept []keptFile,
) (committedMail, error) {
	thread := p.threadMailOf(mail, row.subject)
	threadID, err := chooseThread(ctx, tx, tenant, thread, p.times.subjectThreadWindow)
	if err != nil {
		return committedMail{}, err
	}
	row.threadID = threadID
	id, err := insertMessage(ctx, tx, tenant, row)
	if err != nil {
		return committedMail{}, err
	}
	if err := p.insertParts(ctx, tx, tenant, mail, thread, row, id, kept); err != nil {
		return committedMail{}, err
	}
	holder, err := readKeyHolder(ctx, tx, tenant, mail.box.id, thread.keys.own)
	if err != nil {
		return committedMail{}, err
	}
	if err := dequeueFiles(ctx, tx, tenant, kept); err != nil {
		return committedMail{}, err
	}
	if err := deleteFailedMail(ctx, tx, tenant, mail.box.id, mail.in.ProviderRef); err != nil {
		return committedMail{}, err
	}
	if err := addUsedBytes(ctx, tx, tenant, row.storedBytes); err != nil {
		return committedMail{}, err
	}
	return committedMail{outcome: newRowOutcome(mail), id: id, row: row, idHolder: holder}, nil
}

// insertParts stores the reference, the links, the files and the thread keys of the new mail id inside tx.
func (p *ingester) insertParts(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail parsedMail, thread threadMail, row messageRow,
	id uuid.UUID, kept []keptFile,
) error {
	if err := addProviderRef(ctx, tx, tenant, referenceOf(mail, id)); err != nil {
		return err
	}
	if _, err := linkContacts(ctx, tx, tenant, id, sortedContacts(mail.contacts)); err != nil {
		return err
	}
	files := attachmentRows(mail.message, kept, p.caps.nameMaxBytes)
	if err := insertAttachments(ctx, tx, tenant, id, files); err != nil {
		return err
	}
	if err := claimMailKeys(ctx, tx, tenant, thread, row.threadID, id, p.times.subjectThreadWindow); err != nil {
		return err
	}
	entry := threadEntry{subject: row.subject, subjectKey: subjectKeyOf(mail.message.Subject), at: row.receivedAt}
	return updateThreadSpan(ctx, tx, tenant, row.threadID, entry)
}

// readKeyHolder returns the stored mail holding the key own in the mailbox inside tx, uuid.Nil when own is nil.
func readKeyHolder(ctx context.Context, tx pgx.Tx, tenant, mailboxID uuid.UUID, own *threadKey) (uuid.UUID, error) {
	if own == nil {
		return uuid.Nil, nil
	}
	var holder uuid.UUID
	err := tx.QueryRow(ctx, `SELECT message_id FROM plugin_email.thread_keys
		WHERE tenant_id = $1 AND mailbox_id = $2 AND kind = $3 AND key = $4`,
		tenant, mailboxID, own.kind, own.hash[:],
	).Scan(&holder)
	if err != nil {
		return uuid.Nil, fmt.Errorf("email: read the holder of a message id key: %w", err)
	}
	return holder, nil
}

// linkCopy adds one more copy of the stored mail id inside tx, queues its kept files for deletion and counts its bytes.
func (p *ingester) linkCopy(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail parsedMail, id uuid.UUID, kept []keptFile,
) (Outcome, error) {
	newLinks, err := p.addCopy(ctx, tx, tenant, mail, id)
	if err != nil {
		return "", err
	}
	if err := markDeleting(ctx, tx, tenant, kept); err != nil {
		return "", err
	}
	if err := deleteFailedMail(ctx, tx, tenant, mail.box.id, mail.in.ProviderRef); err != nil {
		return "", err
	}
	grown, err := addParticipants(ctx, tx, tenant, id, mail.participants, p.caps.recipientsMax)
	if err != nil {
		return "", err
	}
	if err := addUsedBytes(ctx, tx, tenant, grown); err != nil {
		return "", err
	}
	return copyOutcome(newLinks), nil
}

// addCopy adds the reference, the links and the thread keys of a copy to the stored mail id in tx, counting new links.
func (p *ingester) addCopy(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail parsedMail, id uuid.UUID,
) (int64, error) {
	if err := addProviderRef(ctx, tx, tenant, referenceOf(mail, id)); err != nil {
		return 0, err
	}
	newLinks, err := linkContacts(ctx, tx, tenant, id, sortedContacts(mail.contacts))
	if err != nil {
		return 0, err
	}
	keys := p.threadKeysOf(mail).mentioned()
	if err := claimThreadKeys(ctx, tx, tenant, id, keys, p.times.subjectThreadWindow); err != nil {
		return 0, err
	}
	if mail.in.SentCopy {
		return newLinks, turnOutbound(ctx, tx, tenant, id)
	}
	return newLinks, nil
}

// countNewLinks counts the contacts of the workspace tenant among contacts the mail id holds no link to yet.
func countNewLinks(
	ctx context.Context, db rowQuerier, tenant, messageID uuid.UUID, contacts []uuid.UUID,
) (int64, error) {
	var newLinks int64
	err := db.QueryRow(ctx, `SELECT count(*) FROM core.contacts c
		WHERE c.tenant_id = $1 AND c.id = ANY($3) AND NOT EXISTS (
			SELECT FROM plugin_email.message_contacts l
			WHERE l.tenant_id = $1 AND l.message_id = $2 AND l.contact_id = c.id)`,
		tenant, messageID, contacts,
	).Scan(&newLinks)
	if err != nil {
		return 0, fmt.Errorf("email: count the new links of a mail: %w", err)
	}
	return newLinks, nil
}

// setAside queues the files of a failed run for deletion and records why a run its caller did not cancel failed.
func (p *ingester) setAside(
	ctx context.Context, tenant uuid.UUID, mail parsedMail, kept []keptFile, failure error,
) (Outcome, error) {
	if err := p.files.queueFilesForDeletion(ctx, tenant, kept); err != nil {
		p.logger.WarnContext(ctx, "email: could not queue the files of a failed mail for deletion",
			"workspace", tenant, "mailbox", mail.box.id, "reason", failureReason(err))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "", failure
	}
	reason := failureReason(failure)
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.times.cleanupTimeout)
	defer cancel()
	if err := recordFailure(recordCtx, p.pool, tenant, mail.box.id, mail.in.ProviderRef, reason, p.now()); err != nil {
		return "", errors.Join(failure, err)
	}
	if reason == permanentDatabaseError {
		return outcomeFailed, nil
	}
	return "", failure
}

// logStored logs by ids the parts a new mail could not read and the stored mail holding its Message-ID.
func (p *ingester) logStored(ctx context.Context, tenant uuid.UUID, written committedMail) {
	if written.row.skippedParts > 0 {
		p.logger.InfoContext(ctx, "email: stored a mail without the parts it could not read",
			"workspace", tenant, "message", written.id, "skipped_parts", written.row.skippedParts)
	}
	if written.idHolder != uuid.Nil && written.idHolder != written.id {
		p.logger.InfoContext(ctx, "email: stored a mail whose Message-ID another stored mail holds",
			"workspace", tenant, "message", written.id, "holder", written.idHolder)
	}
}

// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// querier reads rows on a pool or inside a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// mailView is one mail as a light read answers it, with its first sender in place of its people and no body.
type mailView struct {
	// id names the mail.
	id uuid.UUID
	// threadID names the thread the mail sits in.
	threadID uuid.UUID
	// direction says whether the mailbox received or sent the mail.
	direction string
	// subject is the subject the mail keeps.
	subject string
	// from is the first sender the mail names, nil when it names none.
	from *participant
	// sentAt is when the sender wrote the mail, nil when its Date is missing.
	sentAt *time.Time
	// receivedAt is when the mailbox received the mail.
	receivedAt time.Time
	// snippet is the preview of the mail.
	snippet string
	// bodyState says whether the text is whole, cut or missing behind the headers.
	bodyState string
	// skippedParts counts the parts too broken to read.
	skippedParts int
	// automation names the kind of program that sent the mail, empty for a person.
	automation string
	// encrypted reports an encrypted part.
	encrypted bool
	// signed reports a signed part.
	signed bool
	// winmail reports a winmail.dat part.
	winmail bool
	// inviteMethod is the method of the calendar invite the mail carries, empty for none.
	inviteMethod string
	// rawState says whether the raw mail is kept, and why not when it is not.
	rawState string
	// rawSize is the size of the raw mail.
	rawSize int64
	// storedBytes counts what the mail adds to its workspace, its text and every file it keeps.
	storedBytes int64
}

// threadView is one thread as the inbox lists it, beside its newest mail.
type threadView struct {
	// id names the thread.
	id uuid.UUID
	// subject is the subject of the thread's earliest mail.
	subject string
	// firstMessageAt is when the mailbox received the thread's earliest mail.
	firstMessageAt time.Time
	// lastMessageAt is when the mailbox received the thread's newest mail.
	lastMessageAt time.Time
	// messageCount counts the mails of the thread.
	messageCount int
	// hasAttachments reports a mail of the thread holding a file that is not an inline image.
	hasAttachments bool
	// latest is the mail of the thread the mailbox received last.
	latest mailView
}

// threadPage is one page of the inbox, beside how many threads the workspace holds.
type threadPage struct {
	// items are the threads of the page, newest mail first.
	items []threadView
	// total counts every thread of the workspace.
	total int
}

// mailPage is one page of mail, beside how many mails the whole list holds.
type mailPage struct {
	// items are the mails of the page, newest first.
	items []mailView
	// total counts every mail of the list.
	total int
}

// mailColumns reads the light columns of the mail m, its first sender in place of its people.
const mailColumns = `m.id, m.thread_id, m.direction, m.subject,
	jsonb_path_query_first(m.participants, '$[*] ? (@.role == "from")'), m.sent_at, m.received_at, m.snippet,
	m.body_state, m.skipped_parts, m.automation, m.encrypted, m.signed, m.winmail, m.invite_method, m.raw_state,
	m.raw_size, m.stored_bytes`

// threadSelect reads each thread t beside its count of mail, whether it holds files and its newest mail.
const threadSelect = `SELECT t.id, t.subject, t.first_message_at, t.last_message_at,
		(SELECT count(*) FROM plugin_email.messages c WHERE c.tenant_id = $1 AND c.thread_id = t.id),
		EXISTS (SELECT FROM plugin_email.attachments a
			JOIN plugin_email.messages f ON f.id = a.message_id
			WHERE a.tenant_id = $1 AND f.tenant_id = $1 AND f.thread_id = t.id AND NOT a.inline),
		latest.*
	FROM plugin_email.threads t
	JOIN LATERAL (SELECT ` + mailColumns + ` FROM plugin_email.messages m
		WHERE m.tenant_id = $1 AND m.thread_id = t.id
		ORDER BY m.received_at DESC, m.id DESC
		LIMIT 1) latest ON TRUE`

// mailFields returns where each light column of a mail scans into.
func mailFields(mail *mailView) []any {
	return []any{
		&mail.id, &mail.threadID, &mail.direction, &mail.subject, &mail.from, &mail.sentAt, &mail.receivedAt,
		&mail.snippet, &mail.bodyState, &mail.skippedParts, &mail.automation, &mail.encrypted, &mail.signed,
		&mail.winmail, &mail.inviteMethod, &mail.rawState, &mail.rawSize, &mail.storedBytes,
	}
}

// scanMail reads one mail of a light read.
func scanMail(row pgx.CollectableRow) (mailView, error) {
	var mail mailView
	err := row.Scan(mailFields(&mail)...)
	return mail, err
}

// scanThread reads one thread beside its newest mail.
func scanThread(row pgx.CollectableRow) (threadView, error) {
	var thread threadView
	err := row.Scan(append([]any{
		&thread.id, &thread.subject, &thread.firstMessageAt, &thread.lastMessageAt, &thread.messageCount,
		&thread.hasAttachments,
	}, mailFields(&thread.latest)...)...)
	return thread, err
}

// oneOrNone returns the one row rows holds read by scan, and whether it holds one, its failure naming what it reads.
func oneOrNone[T any](rows pgx.Rows, scan pgx.RowToFunc[T], what string) (T, bool, error) {
	row, err := pgx.CollectExactlyOneRow(rows, scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("email: read %s: %w", what, err)
	}
	return row, true, nil
}

// readCount returns the count statement reads with args, its failure naming what it counts.
func readCount(ctx context.Context, db querier, what, statement string, args ...any) (int, error) {
	rows, _ := db.Query(ctx, statement, args...)
	total, err := pgx.CollectExactlyOneRow(rows, pgx.RowTo[int])
	if err != nil {
		return 0, fmt.Errorf("email: count %s: %w", what, err)
	}
	return total, nil
}

// readInbox returns a page of the threads of the workspace tenant by their newest mail, beside how many it holds.
func readInbox(ctx context.Context, db querier, tenant uuid.UUID, limit, offset int) (threadPage, error) {
	rows, _ := db.Query(ctx, threadSelect+`
		WHERE t.tenant_id = $1
		ORDER BY t.last_message_at DESC, t.id DESC
		LIMIT $2 OFFSET $3`,
		tenant, limit, offset)
	threads, err := pgx.CollectRows(rows, scanThread)
	if err != nil {
		return threadPage{}, fmt.Errorf("email: read the inbox: %w", err)
	}
	total, err := readCount(ctx, db, "the inbox",
		"SELECT count(*) FROM plugin_email.threads WHERE tenant_id = $1", tenant)
	if err != nil {
		return threadPage{}, err
	}
	return threadPage{items: threads, total: total}, nil
}

// readThread returns the thread id names in the workspace tenant, and whether the workspace holds it.
func readThread(ctx context.Context, db querier, tenant, id uuid.UUID) (threadView, bool, error) {
	rows, _ := db.Query(ctx, threadSelect+`
		WHERE t.tenant_id = $1 AND t.id = $2`,
		tenant, id)
	return oneOrNone(rows, scanThread, "a thread")
}

// readThreadMails returns a page of the mail of one thread of the workspace tenant, newest first.
func readThreadMails(
	ctx context.Context, db querier, tenant, thread uuid.UUID, limit, offset int,
) ([]mailView, error) {
	rows, _ := db.Query(ctx, `SELECT `+mailColumns+` FROM plugin_email.messages m
		WHERE m.tenant_id = $1 AND m.thread_id = $2
		ORDER BY m.received_at DESC, m.id DESC
		LIMIT $3 OFFSET $4`,
		tenant, thread, limit, offset)
	mails, err := pgx.CollectRows(rows, scanMail)
	if err != nil {
		return nil, fmt.Errorf("email: read the mail of a thread: %w", err)
	}
	return mails, nil
}

// readThreadMail returns the mail id names in one thread of the workspace tenant, and whether the thread holds it.
func readThreadMail(ctx context.Context, db querier, tenant, thread, id uuid.UUID) (mailView, bool, error) {
	rows, _ := db.Query(ctx, `SELECT `+mailColumns+` FROM plugin_email.messages m
		WHERE m.tenant_id = $1 AND m.thread_id = $2 AND m.id = $3`,
		tenant, thread, id)
	return oneOrNone(rows, scanMail, "a mail of a thread")
}

// readMail returns the mail id names in the workspace tenant, and whether the workspace holds it.
func readMail(ctx context.Context, db querier, tenant, id uuid.UUID) (mailView, bool, error) {
	rows, _ := db.Query(ctx, `SELECT `+mailColumns+` FROM plugin_email.messages m
		WHERE m.tenant_id = $1 AND m.id = $2`,
		tenant, id)
	return oneOrNone(rows, scanMail, "a mail")
}

// readContactMails returns a page of the mail linked to one contact of the workspace tenant, newest first.
func readContactMails(
	ctx context.Context, db querier, tenant, contact uuid.UUID, limit, offset int,
) (mailPage, error) {
	rows, _ := db.Query(ctx, `SELECT `+mailColumns+` FROM plugin_email.message_contacts l
		JOIN plugin_email.messages m ON m.id = l.message_id
		WHERE l.tenant_id = $1 AND m.tenant_id = $1 AND l.contact_id = $2
		ORDER BY m.received_at DESC, m.id DESC
		LIMIT $3 OFFSET $4`,
		tenant, contact, limit, offset)
	mails, err := pgx.CollectRows(rows, scanMail)
	if err != nil {
		return mailPage{}, fmt.Errorf("email: read the mail of a contact: %w", err)
	}
	total, err := readCount(ctx, db, "the mail of a contact",
		"SELECT count(*) FROM plugin_email.message_contacts WHERE tenant_id = $1 AND contact_id = $2", tenant, contact)
	if err != nil {
		return mailPage{}, err
	}
	return mailPage{items: mails, total: total}, nil
}

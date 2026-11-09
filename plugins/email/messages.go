// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gopherium/alphone/plugins/email/message"
)

const (
	// inbound is the direction of a mail the mailbox received.
	inbound = "inbound"
	// outbound is the direction of a mail the mailbox sent.
	outbound = "outbound"
)

const (
	// bodyWhole marks a mail whose text and HTML are kept whole.
	bodyWhole = "whole"
	// bodyCut marks a mail whose text and HTML were cut at the body cap.
	bodyCut = "cut"
	// bodyHeadersOnly marks a mail too large or too broken to read past its headers.
	bodyHeadersOnly = "headers_only"
)

const (
	// fileStored marks a raw copy or an attachment kept in the file store.
	fileStored = "stored"
	// fileTooLarge marks a raw copy or an attachment over its byte cap, kept with no file.
	fileTooLarge = "too_large"
	// fileOverLimit marks a raw copy or an attachment past the workspace limit, kept with no file.
	fileOverLimit = "over_limit"
	// rawNone marks a mail that has no raw copy to keep.
	rawNone = "none"
)

// errAlreadyStored reports a mail whose dedupe key the mailbox already holds.
var errAlreadyStored = errors.New("email: the mail is already stored")

// rowQuerier reads one row on a pool or inside a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// participant is one address a mail names, beside the field that names it.
type participant struct {
	// Role is the field that names the address, such as from, to or cc.
	Role string `json:"role"`
	// Name is the display name the field gives, empty when it gives none.
	Name string `json:"name"`
	// Email is the address in the form a contact stores.
	Email string `json:"email"`
}

// messageRow is one mail as its row in plugin_email.messages holds it.
type messageRow struct {
	// mailboxID names the mailbox the mail reached.
	mailboxID uuid.UUID
	// threadID names the thread the mail joined.
	threadID uuid.UUID
	// providerThreadID is the provider's own thread id, empty for none.
	providerThreadID string
	// dedupeKey names the mail inside its mailbox, nil for none.
	dedupeKey []byte
	// internetMessageID is the id the Message-ID field holds, empty for none.
	internetMessageID string
	// inReplyToIDs are the ids the In-Reply-To field holds.
	inReplyToIDs []string
	// referenceIDs are the ids the References field holds.
	referenceIDs []string
	// direction says whether the mailbox received or sent the mail.
	direction string
	// subject is the subject the mail keeps.
	subject string
	// participants are the addresses the mail names, each beside its field.
	participants []participant
	// sentAt is when the sender wrote the mail, zero when its Date is missing.
	sentAt time.Time
	// receivedAt is when the mailbox received the mail.
	receivedAt time.Time
	// bodyText is the plain text the mail keeps.
	bodyText string
	// safeHTML is the HTML the mail keeps, safe to show.
	safeHTML string
	// snippet is the preview of the mail.
	snippet string
	// bodyState says whether the text is whole, cut or missing behind the headers.
	bodyState string
	// skippedParts counts the parts too broken to read.
	skippedParts int
	// automation names the kind of program that sent the mail, empty for a person.
	automation message.Automation
	// encrypted reports an encrypted part.
	encrypted bool
	// signed reports a signed part.
	signed bool
	// winmail reports a winmail.dat part.
	winmail bool
	// inviteMethod is the method of the calendar invite the mail carries, empty for none.
	inviteMethod string
	// rawFileID names the file that keeps the raw mail, uuid.Nil for none.
	rawFileID uuid.UUID
	// rawSize is the size of the raw mail.
	rawSize int64
	// rawSHA256 is the hash of the raw mail, nil for none.
	rawSHA256 []byte
	// rawState says whether the raw mail is kept, and why not when it is not.
	rawState string
	// storedBytes is what the mail adds to the bytes its workspace keeps.
	storedBytes int64
}

// providerRef is the provider's own id for one copy of a stored mail.
type providerRef struct {
	// mailboxID names the mailbox the copy sits in.
	mailboxID uuid.UUID
	// ref is the provider's id of the copy, with the prefix its caller picks.
	ref string
	// messageID names the stored mail the copy is of.
	messageID uuid.UUID
	// sentCopy reports a copy read from the mailbox's Sent folder.
	sentCopy bool
}

// attachmentRow is one file of a mail as its row in plugin_email.attachments holds it.
type attachmentRow struct {
	// position is the place of the file among the files of its mail.
	position int
	// filename is the name the mail gives the file.
	filename string
	// contentType is the media type the file is kept under.
	contentType string
	// size is the size of the file.
	size int64
	// sha256 is the hash of the file.
	sha256 []byte
	// inline reports an image the HTML body shows.
	inline bool
	// contentID is the Content-ID of the file, empty for none.
	contentID string
	// fileID names the kept file, uuid.Nil when no file is kept.
	fileID uuid.UUID
	// state says whether the file is kept, and why not when it is not.
	state string
}

// nullableID returns id as a uuid the database stores as NULL when it is uuid.Nil.
func nullableID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}

// insertMessage stores row in the workspace tenant inside tx and returns its new id, errAlreadyStored for a held key.
func insertMessage(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, row messageRow) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("email: make a message id: %w", err)
	}
	var stored uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO plugin_email.messages (id, tenant_id, mailbox_id, thread_id,
			provider_thread_id, dedupe_key, internet_message_id, in_reply_to_ids, reference_ids, direction, subject,
			participants, sent_at, received_at, body_text, safe_html, snippet, body_state, skipped_parts, automation,
			encrypted, signed, winmail, invite_method, raw_file_id, raw_size, raw_sha256, raw_state, stored_bytes)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), COALESCE($8::text[], '{}'),
			COALESCE($9::text[], '{}'), $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24,
			$25, $26, $27, $28, $29)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		id, tenant, row.mailboxID, row.threadID, row.providerThreadID, row.dedupeKey, row.internetMessageID,
		row.inReplyToIDs, row.referenceIDs, row.direction, row.subject, row.participants,
		pgtype.Timestamptz{Time: row.sentAt, Valid: !row.sentAt.IsZero()}, row.receivedAt, row.bodyText,
		row.safeHTML, row.snippet, row.bodyState, row.skippedParts, string(row.automation), row.encrypted, row.signed,
		row.winmail, row.inviteMethod, nullableID(row.rawFileID), row.rawSize, row.rawSHA256, row.rawState,
		row.storedBytes,
	).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errAlreadyStored
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("email: insert message: %w", err)
	}
	return id, nil
}

// addProviderRef stores ref in the workspace tenant inside tx, leaving a reference the mailbox holds as it is.
func addProviderRef(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ref providerRef) error {
	_, err := tx.Exec(ctx, `INSERT INTO plugin_email.provider_refs
			(tenant_id, mailbox_id, provider_ref, message_id, sent_copy)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, mailbox_id, provider_ref) DO NOTHING`,
		tenant, ref.mailboxID, ref.ref, ref.messageID, ref.sentCopy)
	if err != nil {
		return fmt.Errorf("email: add provider ref: %w", err)
	}
	return nil
}

// readMessageIDByProviderRef returns the mail a provider ref of the mailbox names, wrapping pgx.ErrNoRows for none.
func readMessageIDByProviderRef(
	ctx context.Context, db rowQuerier, tenant, mailboxID uuid.UUID, ref string,
) (uuid.UUID, error) {
	return readMessageID(ctx, db, "a provider ref", `SELECT message_id FROM plugin_email.provider_refs
		WHERE tenant_id = $1 AND mailbox_id = $2 AND provider_ref = $3`,
		tenant, mailboxID, ref)
}

// readMessageIDByDedupeKey returns the mail a dedupe key of the mailbox names, wrapping pgx.ErrNoRows for none.
func readMessageIDByDedupeKey(
	ctx context.Context, db rowQuerier, tenant, mailboxID uuid.UUID, key []byte,
) (uuid.UUID, error) {
	return readMessageID(ctx, db, "a dedupe key", `SELECT id FROM plugin_email.messages
		WHERE tenant_id = $1 AND mailbox_id = $2 AND dedupe_key = $3`,
		tenant, mailboxID, key)
}

// readMessageID returns the mail id statement reads with args, its failure naming the value it looked up by.
func readMessageID(ctx context.Context, db rowQuerier, value, statement string, args ...any) (uuid.UUID, error) {
	var id uuid.UUID
	if err := db.QueryRow(ctx, statement, args...).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("email: read the mail of %s: %w", value, err)
	}
	return id, nil
}

// linkContacts links a mail to each contact of the workspace tenant among contacts and returns how many links are new.
func linkContacts(ctx context.Context, tx pgx.Tx, tenant, messageID uuid.UUID, contacts []uuid.UUID) (int64, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO plugin_email.message_contacts (tenant_id, message_id, contact_id)
		SELECT $1, $2, c.id FROM core.contacts c WHERE c.tenant_id = $1 AND c.id = ANY($3)
		ON CONFLICT DO NOTHING`,
		tenant, messageID, contacts)
	if err != nil {
		return 0, fmt.Errorf("email: link contacts: %w", err)
	}
	return tag.RowsAffected(), nil
}

// insertAttachments stores each file of a mail in the workspace tenant inside tx, in one round trip.
func insertAttachments(ctx context.Context, tx pgx.Tx, tenant, messageID uuid.UUID, files []attachmentRow) error {
	batch := &pgx.Batch{}
	for _, file := range files {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("email: make an attachment id: %w", err)
		}
		batch.Queue(`INSERT INTO plugin_email.attachments (id, tenant_id, message_id, position, filename,
				content_type, size, sha256, inline, content_id, file_id, state)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			id, tenant, messageID, file.position, file.filename, file.contentType, file.size, file.sha256, file.inline,
			file.contentID, nullableID(file.fileID), file.state)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("email: insert attachments: %w", err)
	}
	return nil
}

// participantsSize returns the bytes the JSON of participants takes in the stored bytes of a mail.
func participantsSize(participants []participant) int64 {
	encoded, _ := json.Marshal(participants)
	return int64(len(encoded))
}

// mergedParticipants returns each of held and then of added once, in that order, keeping at most limit.
func mergedParticipants(held, added []participant, limit int) []participant {
	merged := make([]participant, 0, len(held)+len(added))
	for _, entry := range slices.Concat(held, added) {
		if !slices.Contains(merged, entry) {
			merged = append(merged, entry)
		}
	}
	return merged[:min(len(merged), limit)]
}

// addParticipants appends to a mail each of added it lacks, keeping at most limit, and returns the bytes it adds.
func addParticipants(
	ctx context.Context, tx pgx.Tx, tenant, messageID uuid.UUID, added []participant, limit int,
) (int64, error) {
	var held []participant
	err := tx.QueryRow(ctx, `SELECT participants FROM plugin_email.messages WHERE tenant_id = $1 AND id = $2`,
		tenant, messageID).Scan(&held)
	if err != nil {
		return 0, fmt.Errorf("email: add participants: %w", err)
	}
	kept := mergedParticipants(held, added, limit)
	grown := participantsSize(kept) - participantsSize(held)
	_, err = tx.Exec(ctx, `UPDATE plugin_email.messages SET participants = $3, stored_bytes = stored_bytes + $4
		WHERE tenant_id = $1 AND id = $2`,
		tenant, messageID, kept, grown)
	if err != nil {
		return 0, fmt.Errorf("email: add participants: %w", err)
	}
	return grown, nil
}

// turnOutbound marks a mail of the workspace tenant as one the mailbox sent.
func turnOutbound(ctx context.Context, tx pgx.Tx, tenant, messageID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE plugin_email.messages SET direction = 'outbound'
		WHERE tenant_id = $1 AND id = $2`,
		tenant, messageID)
	if err != nil {
		return fmt.Errorf("email: turn outbound: %w", err)
	}
	return nil
}

// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type store struct {
	pool *pgxpool.Pool
}

type conversationRow struct {
	ID                 uuid.UUID `db:"id"`
	ContactID          uuid.UUID `db:"contact_id"`
	ContactName        string    `db:"contact_name"`
	ExternalID         string    `db:"external_id"`
	Status             string    `db:"status"`
	LastActivityAt     time.Time `db:"last_activity_at"`
	LastMessagePreview *string   `db:"last_message_preview"`
}

type messageRow struct {
	ID          uuid.UUID `db:"id"`
	ExternalID  string    `db:"external_id"`
	Direction   string    `db:"direction"`
	Content     string    `db:"content"`
	ContentType string    `db:"content_type"`
	SentAt      time.Time `db:"sent_at"`
}

// listConversations returns up to limit conversations with their contact names and a preview of their
// newest message, most recently active first.
func (s *store) listConversations(ctx context.Context, limit int) ([]conversationRow, error) {
	rows, _ := s.pool.Query(ctx, `
		SELECT conv.id, conv.contact_id, c.name AS contact_name, conv.external_id, conv.status,
			conv.last_activity_at, last_message.preview AS last_message_preview
		FROM plugin_whatsapp.conversations conv
		JOIN core.contacts c ON c.id = conv.contact_id
		LEFT JOIN LATERAL (
			SELECT CASE
				WHEN m.content_type = 'text' OR m.content <> '' THEN LEFT(m.content, 140)
				WHEN m.content_type = 'image' THEN '[photo]'
				WHEN m.content_type = 'audio' THEN '[voice message]'
				WHEN m.content_type = 'video' THEN '[video]'
				WHEN m.content_type = 'document' THEN '[document]'
				WHEN m.content_type = 'sticker' THEN '[sticker]'
				WHEN m.content_type = 'location' THEN '[location]'
				WHEN m.content_type = 'contacts' THEN '[contact]'
				WHEN m.content_type = 'reaction' THEN '[reaction]'
				ELSE '[unsupported]'
			END AS preview
			FROM plugin_whatsapp.messages m
			WHERE m.conversation_id = conv.id
			ORDER BY m.sent_at DESC, m.id DESC
			LIMIT 1
		) last_message ON TRUE
		ORDER BY conv.last_activity_at DESC
		LIMIT $1`,
		limit,
	)
	conversations, err := pgx.CollectRows(rows, pgx.RowToStructByName[conversationRow])
	if err != nil {
		return nil, fmt.Errorf("whatsapp: list conversations: %w", err)
	}
	return conversations, nil
}

// listMessages returns up to limit messages for the given conversation, oldest first.
func (s *store) listMessages(ctx context.Context, conversationID uuid.UUID, limit int) ([]messageRow, error) {
	rows, _ := s.pool.Query(ctx, `
		SELECT id, external_id, direction, content, content_type, sent_at
		FROM plugin_whatsapp.messages
		WHERE conversation_id = $1
		ORDER BY sent_at ASC, id ASC
		LIMIT $2`,
		conversationID, limit,
	)
	messages, err := pgx.CollectRows(rows, pgx.RowToStructByName[messageRow])
	if err != nil {
		return nil, fmt.Errorf("whatsapp: list messages: %w", err)
	}
	return messages, nil
}

// upsertConversation inserts a conversation for the contact or updates its last activity time, returning its id.
func upsertConversation(
	ctx context.Context,
	exec pgxExecutor,
	contactID uuid.UUID,
	externalID string,
	activityAt time.Time,
) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("whatsapp: generate conversation id: %w", err)
	}
	var conversationID uuid.UUID
	err = exec.QueryRow(ctx, `
		INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status, last_activity_at, created_at)
		VALUES ($1, $2, 'whatsapp', $3, 'open', $4, $5)
		ON CONFLICT (external_id) DO UPDATE
		SET last_activity_at = GREATEST(plugin_whatsapp.conversations.last_activity_at, EXCLUDED.last_activity_at)
		RETURNING id`,
		id, contactID, externalID, activityAt, time.Now().UTC(),
	).Scan(&conversationID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("whatsapp: upsert conversation: %w", err)
	}
	return conversationID, nil
}

// persistInbound stores an inbound message, its conversation, and any pending
// media in one transaction, reporting the conversation id and whether the
// message is new rather than a redelivery.
func (s *store) persistInbound(ctx context.Context, contactID uuid.UUID, m inboundMessage) (uuid.UUID, bool, error) {
	var conversationID uuid.UUID
	var stored bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		conversationID, err = upsertConversation(ctx, tx, contactID, m.sender, m.sentAt)
		if err != nil {
			return err
		}
		var messageID uuid.UUID
		messageID, stored, err = insertMessage(ctx, tx, conversationID, m)
		if err != nil {
			return err
		}
		if !stored || m.media == nil {
			return nil
		}
		return insertMediaPending(ctx, tx, messageID, *m.media)
	})
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("whatsapp: persist inbound: %w", err)
	}
	return conversationID, stored, nil
}

type outboundMessage struct {
	externalID string
	content    string
	sentAt     time.Time
	raw        json.RawMessage
}

// conversationExternalID returns the external id of the conversation with the given id.
func (s *store) conversationExternalID(ctx context.Context, conversationID uuid.UUID) (string, error) {
	var externalID string
	err := s.pool.QueryRow(ctx,
		`SELECT external_id FROM plugin_whatsapp.conversations WHERE id = $1`,
		conversationID,
	).Scan(&externalID)
	if err != nil {
		return "", fmt.Errorf("whatsapp: load conversation: %w", err)
	}
	return externalID, nil
}

// appendOutboundMessage stores an outbound message and advances the conversation's last activity time,
// returning the stored row.
func (s *store) appendOutboundMessage(
	ctx context.Context,
	conversationID uuid.UUID,
	m outboundMessage,
) (messageRow, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return messageRow{}, fmt.Errorf("whatsapp: generate message id: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		WITH inserted AS (
			INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
				content_type, sent_at, raw, created_at)
			VALUES ($1, $2, $3, 'outbound', $4, 'text', $5, $6, $7)
		)
		UPDATE plugin_whatsapp.conversations
		SET last_activity_at = GREATEST(last_activity_at, $5)
		WHERE id = $2`,
		id, conversationID, m.externalID, m.content, m.sentAt, m.raw, time.Now().UTC(),
	)
	if err != nil {
		return messageRow{}, fmt.Errorf("whatsapp: store outbound message: %w", err)
	}
	return messageRow{
		ID:          id,
		ExternalID:  m.externalID,
		Direction:   "outbound",
		Content:     m.content,
		ContentType: "text",
		SentAt:      m.sentAt,
	}, nil
}

// insertMessage stores an inbound message, reporting its id and whether it
// was newly stored rather than deduplicated by external id.
func insertMessage(
	ctx context.Context,
	exec pgxExecutor,
	conversationID uuid.UUID,
	m inboundMessage,
) (uuid.UUID, bool, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("whatsapp: generate message id: %w", err)
	}
	var insertedID uuid.UUID
	err = exec.QueryRow(ctx, `
		INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at)
		VALUES ($1, $2, $3, 'inbound', $4, $5, $6, $7, $8)
		ON CONFLICT (external_id) DO NOTHING
		RETURNING id`,
		id, conversationID, m.externalID, m.content, m.contentType, m.sentAt, m.raw, time.Now().UTC(),
	).Scan(&insertedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("whatsapp: insert message: %w", err)
	}
	return insertedID, true, nil
}

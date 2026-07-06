// SPDX-License-Identifier: AGPL-3.0-or-later

package whatsapp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type store struct {
	pool *pgxpool.Pool
}

func (s *store) upsertConversation(ctx context.Context, contactID uuid.UUID, externalID string, activityAt time.Time) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("whatsapp: generate conversation id: %w", err)
	}
	var conversationID uuid.UUID
	err = s.pool.QueryRow(ctx, `
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

func (s *store) insertMessage(ctx context.Context, conversationID uuid.UUID, m inboundMessage) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("whatsapp: generate message id: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content, content_type, sent_at, raw, created_at)
		VALUES ($1, $2, $3, 'inbound', $4, 'text', $5, $6, $7)
		ON CONFLICT (external_id) DO NOTHING`,
		id, conversationID, m.externalID, m.text, m.sentAt, m.raw, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("whatsapp: insert message: %w", err)
	}
	return nil
}

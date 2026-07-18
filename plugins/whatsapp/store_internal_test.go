// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestListMessagesOrdersSameSecondMessagesByID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx := t.Context()
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)`,
		contactID, "María Pérez", time.Now().UTC(),
	); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	conversationID := uuid.Must(uuid.NewV7())
	sentAt := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status,
			last_activity_at, created_at)
		VALUES ($1, $2, 'whatsapp', '184467235', 'open', $3, $3)`,
		conversationID, contactID, sentAt,
	); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	ordered := []uuid.UUID{uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())}
	for i, id := range []uuid.UUID{ordered[1], ordered[2], ordered[0]} {
		if _, err := p.pool.Exec(ctx,
			`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
				content_type, sent_at, raw, created_at)
			VALUES ($1, $2, $3, 'inbound', 'hola', 'text', $4, '{}', $5)`,
			id, conversationID, fmt.Sprintf("wamid.%d", i), sentAt, time.Now().UTC(),
		); err != nil {
			t.Fatalf("inserting message %d: %v", i, err)
		}
	}

	messages, err := p.store.listMessages(ctx, conversationID, 50)
	if err != nil {
		t.Fatalf("listMessages() error = %v, want nil", err)
	}

	if got, want := len(messages), 3; got != want {
		t.Fatalf("len(messages) = %d, want %d", got, want)
	}
	for i, want := range ordered {
		if messages[i].ID != want {
			t.Fatalf("messages[%d].ID = %s, want %s", i, messages[i].ID, want)
		}
	}
}

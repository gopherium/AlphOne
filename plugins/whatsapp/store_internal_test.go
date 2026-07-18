// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func seedPreviewConversation(
	t *testing.T, p *Plugin, contactID uuid.UUID, externalID, contentType, content string,
) uuid.UUID {
	t.Helper()
	ctx := t.Context()
	now := time.Now().UTC()
	conversationID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status,
			last_activity_at, created_at)
		VALUES ($1, $2, 'whatsapp', $3, 'open', $4, $4)`,
		conversationID, contactID, externalID, now,
	); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at)
		VALUES ($1, $2, $3, 'inbound', $4, $5, $6, '{}', $6)`,
		uuid.Must(uuid.NewV7()), conversationID, externalID, content, contentType, now,
	); err != nil {
		t.Fatalf("inserting message: %v", err)
	}
	return conversationID
}

func TestListConversationsPreviewsEveryContentType(t *testing.T) {
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

	tests := map[string]struct {
		contentType string
		content     string
		want        string
	}{
		"text":                  {contentType: "text", content: "hola", want: "hola"},
		"long text truncated":   {contentType: "text", content: strings.Repeat("a", 200), want: strings.Repeat("a", 140)},
		"image with caption":    {contentType: "image", content: "la factura", want: "la factura"},
		"image without caption": {contentType: "image", content: "", want: "[photo]"},
		"audio":                 {contentType: "audio", content: "", want: "[voice message]"},
		"video":                 {contentType: "video", content: "", want: "[video]"},
		"document":              {contentType: "document", content: "", want: "[document]"},
		"sticker":               {contentType: "sticker", content: "", want: "[sticker]"},
		"location":              {contentType: "location", content: "Museo del Prado", want: "Museo del Prado"},
		"nameless contact card": {contentType: "contacts", content: "", want: "[contact]"},
		"reaction":              {contentType: "reaction", content: "👍", want: "👍"},
		"reaction removal":      {contentType: "reaction", content: "", want: "[reaction]"},
		"unsupported":           {contentType: "unsupported", content: "", want: "[unsupported]"},
		"unknown future type":   {contentType: "poll", content: "", want: "[unsupported]"},
	}

	wantByConversation := make(map[uuid.UUID]string, len(tests))
	index := 0
	for _, tc := range tests {
		externalID := fmt.Sprintf("wamid.preview-%d", index)
		index++
		conversationID := seedPreviewConversation(t, p, contactID, externalID, tc.contentType, tc.content)
		wantByConversation[conversationID] = tc.want
	}

	rows, err := p.store.listConversations(ctx, 50)
	if err != nil {
		t.Fatalf("listConversations() error = %v, want nil", err)
	}

	if got, want := len(rows), len(tests); got != want {
		t.Fatalf("len(rows) = %d, want %d", got, want)
	}
	for _, row := range rows {
		want := wantByConversation[row.ID]
		if row.LastMessagePreview == nil {
			t.Errorf("preview for %s = nil, want %q", row.ID, want)
			continue
		}
		if *row.LastMessagePreview != want {
			t.Errorf("preview = %q, want %q", *row.LastMessagePreview, want)
		}
	}
}

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

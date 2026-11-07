// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// seedPreviewConversation stores one conversation with a single inbound message and returns its id.
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

// storedThread stores a conversation whose messages carry the given sent times and returns it with their ids in order.
func storedThread(t *testing.T, p *Plugin, sentAt ...time.Time) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)`,
		contactID, "Maria Perez", time.Now().UTC(),
	); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	conversationID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status,
			last_activity_at, created_at)
		VALUES ($1, $2, 'whatsapp', $3, 'open', now(), now())`,
		conversationID, contactID, conversationID.String(),
	); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	ids := make([]uuid.UUID, len(sentAt))
	for i, at := range sentAt {
		ids[i] = uuid.Must(uuid.NewV7())
		if _, err := p.pool.Exec(ctx,
			`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
				content_type, sent_at, raw, created_at)
			VALUES ($1, $2, $3, 'inbound', $4, 'text', $5, '{}', now())`,
			ids[i], conversationID, ids[i].String(), fmt.Sprintf("message %d", i+1), at,
		); err != nil {
			t.Fatalf("inserting message %d: %v", i+1, err)
		}
	}
	return conversationID, ids
}

// minutesApart returns count sent times one minute apart.
func minutesApart(count int) []time.Time {
	first := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	times := make([]time.Time, count)
	for i := range times {
		times[i] = first.Add(time.Duration(i) * time.Minute)
	}
	return times
}

// sameSecond returns count sent times that all fall in one second.
func sameSecond(count int) []time.Time {
	times := make([]time.Time, count)
	for i := range times {
		times[i] = time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	}
	return times
}

// wantMessages fails unless rows hold exactly the given ids in that order.
func wantMessages(t *testing.T, rows []messageRow, want ...uuid.UUID) {
	t.Helper()
	got := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		got[i] = row.ID
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("messages = %v, want %v", got, want)
	}
}

// pageBefore lists up to two messages of the conversation that come before the given message.
func pageBefore(t *testing.T, p *Plugin, conversationID, before uuid.UUID) []messageRow {
	t.Helper()
	rows, err := p.store.listMessages(t.Context(), conversationID, 2, &before)
	if err != nil {
		t.Fatalf("listMessages() error = %v, want nil", err)
	}
	return rows
}

// oldestOf returns the message the store names as the oldest of the conversation.
func oldestOf(t *testing.T, p *Plugin, conversationID uuid.UUID) *uuid.UUID {
	t.Helper()
	oldest, err := p.store.oldestMessageID(t.Context(), conversationID)
	if err != nil {
		t.Fatalf("oldestMessageID() error = %v, want nil", err)
	}
	return oldest
}

// wantOldest fails unless oldest names the message want.
func wantOldest(t *testing.T, oldest *uuid.UUID, want uuid.UUID) {
	t.Helper()
	if oldest == nil || *oldest != want {
		t.Errorf("oldestMessageID() = %v, want %s", oldest, want)
	}
}

func TestListMessagesAnswersThePageBeforeTheGivenMessage(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	conversationID, ids := storedThread(t, p, minutesApart(5)...)

	wantMessages(t, pageBefore(t, p, conversationID, ids[3]), ids[1], ids[2])
}

func TestListMessagesOrdersSameSecondPagesByID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	conversationID, ids := storedThread(t, p, sameSecond(4)...)

	wantMessages(t, pageBefore(t, p, conversationID, ids[2]), ids[0], ids[1])
}

func TestListMessagesBeforeAnUnknownMessageIsEmpty(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	conversationID, _ := storedThread(t, p, minutesApart(3)...)

	wantMessages(t, pageBefore(t, p, conversationID, uuid.Must(uuid.NewV7())))
}

func TestListMessagesBeforeAMessageOfAnotherConversationIsEmpty(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	conversationID, _ := storedThread(t, p, minutesApart(3)...)
	_, others := storedThread(t, p, minutesApart(3)...)

	wantMessages(t, pageBefore(t, p, conversationID, others[2]))
}

func TestOldestMessageIDNamesTheFirstMessageSentEvenWhenItArrivedLate(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	times := minutesApart(3)
	conversationID, ids := storedThread(t, p, times[1], times[0], times[2])

	wantOldest(t, oldestOf(t, p, conversationID), ids[1])
}

func TestOldestMessageIDBreaksASameSecondTieByID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	conversationID, ids := storedThread(t, p, sameSecond(3)...)

	wantOldest(t, oldestOf(t, p, conversationID), ids[0])
}

func TestOldestMessageIDIsNilForAConversationWithNoMessages(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	conversationID, _ := storedThread(t, p)

	if oldest := oldestOf(t, p, conversationID); oldest != nil {
		t.Errorf("oldestMessageID() = %s, want nil", *oldest)
	}
}

func TestGetConversationReportsAReadFailure(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := p.store.getConversation(ctx, uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Fatal("getConversation() error = nil, want the read failure reported")
	}
	if !strings.Contains(err.Error(), "whatsapp: get conversation") {
		t.Errorf("getConversation() error = %v, want it to name the read", err)
	}
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
		"text":                  {contentType: "text", content: "hello", want: "hello"},
		"long text truncated":   {contentType: "text", content: strings.Repeat("a", 200), want: strings.Repeat("a", 140)},
		"image with caption":    {contentType: "image", content: "the invoice", want: "the invoice"},
		"image without caption": {contentType: "image", content: "", want: "[photo]"},
		"audio":                 {contentType: "audio", content: "", want: "[voice message]"},
		"video":                 {contentType: "video", content: "", want: "[video]"},
		"document":              {contentType: "document", content: "", want: "[document]"},
		"sticker":               {contentType: "sticker", content: "", want: "[sticker]"},
		"location":              {contentType: "location", content: "British Museum", want: "British Museum"},
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

	rows, err := p.store.listConversations(ctx, 50, nil, "")
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
			VALUES ($1, $2, $3, 'inbound', 'hello', 'text', $4, '{}', $5)`,
			id, conversationID, fmt.Sprintf("wamid.%d", i), sentAt, time.Now().UTC(),
		); err != nil {
			t.Fatalf("inserting message %d: %v", i, err)
		}
	}

	messages, err := p.store.listMessages(ctx, conversationID, 50, nil)
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

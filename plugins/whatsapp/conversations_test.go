// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/plugins/whatsapp"
)

// ingestEvent delivers one inbound text through the webhook.
func ingestEvent(t *testing.T, routes http.Handler, wamid, waID, name, timestamp, text string) {
	t.Helper()
	body := eventBody(wamid, waID, name, timestamp, text)
	if recorder := postEvent(t, routes, sign("app-secret", body), body); recorder.Code != http.StatusOK {
		t.Fatalf("ingesting %s status = %d, want %d", wamid, recorder.Code, http.StatusOK)
	}
}

// listConversations reads the conversations through the graph resolver.
func listConversations(
	t *testing.T, p *whatsapp.Plugin, limit *int,
) []*model.WhatsAppConversation {
	t.Helper()
	conversations, err := p.QueryResolvers().WhatsAppConversations(t.Context(), limit)
	if err != nil {
		t.Fatalf("WhatsAppConversations() error = %v, want nil", err)
	}
	return conversations
}

// listMessages reads one conversation's messages through the graph resolver.
func listMessages(
	t *testing.T, p *whatsapp.Plugin, id uuid.UUID,
) []*model.WhatsAppMessage {
	t.Helper()
	messages, err := p.WhatsAppConversationResolvers().Messages(
		t.Context(), &model.WhatsAppConversation{ID: id}, nil)
	if err != nil {
		t.Fatalf("Messages() error = %v, want nil", err)
	}
	return messages
}

func TestListConversationsOrdersByRecentActivity(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	ingestEvent(t, routes, "wamid.2", "555000111", "John Doe", "1751791100", "hey")

	got := listConversations(t, p, nil)

	if len(got) != 2 {
		t.Fatalf("conversations = %d, want 2", len(got))
	}
	if got[0].Contact.Name != "John Doe" || got[1].Contact.Name != "María Pérez" {
		t.Errorf("order = [%q, %q], want most recent first [%q, %q]",
			got[0].Contact.Name, got[1].Contact.Name, "John Doe", "María Pérez")
	}
	if got[0].Status != "open" || got[0].ExternalID != "555000111" {
		t.Errorf("conversation = %+v, want status open and external id 555000111", got[0])
	}
	if got[0].LastActivityAt.Location() != time.UTC {
		t.Errorf("lastActivityAt location = %v, want UTC", got[0].LastActivityAt.Location())
	}
}

func TestListConversationsIncludeLastMessagePreview(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	ingestEvent(t, routes, "wamid.2", "184467235", "María Pérez", "1751791100", "how are you?")
	ingestEvent(t, routes, "wamid.3", "555000111", "John Doe", "1751791200", strings.Repeat("é", 200))

	got := listConversations(t, p, nil)

	if len(got) != 2 {
		t.Fatalf("conversations = %d, want 2", len(got))
	}
	if got[1].LastMessagePreview == nil || *got[1].LastMessagePreview != "how are you?" {
		t.Errorf("preview = %v, want the newest message of the conversation", got[1].LastMessagePreview)
	}
	if got[0].LastMessagePreview == nil || utf8.RuneCountInString(*got[0].LastMessagePreview) != 140 {
		t.Errorf("preview = %v, want the long message truncated to 140 characters", got[0].LastMessagePreview)
	}
}

func TestListConversationsPreviewPrefersTheLatestOfTiedTimestamps(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "first")
	ingestEvent(t, routes, "wamid.2", "184467235", "María Pérez", "1751791000", "second")

	got := listConversations(t, p, nil)

	if len(got) != 1 {
		t.Fatalf("conversations = %d, want 1", len(got))
	}
	if got[0].LastMessagePreview == nil || *got[0].LastMessagePreview != "second" {
		t.Errorf("preview = %v, want the later-ingested message of the tied pair", got[0].LastMessagePreview)
	}
}

func TestListConversationsWithoutMessagesHasNullPreview(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	ingestEvent(t, routes, "wamid.1", "555000111", "John Doe", "1751791100", "stolen id")

	got := listConversations(t, p, nil)

	if len(got) != 2 {
		t.Fatalf("conversations = %d, want 2", len(got))
	}
	if got[0].Contact.Name != "John Doe" || got[0].LastMessagePreview != nil {
		t.Errorf("conversation = %+v, want John Doe with a null preview for a message-less conversation", got[0])
	}
}

func TestListConversationsEmptyIsAList(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)

	got := listConversations(t, p, nil)

	if got == nil || len(got) != 0 {
		t.Errorf("conversations = %v, want an empty list, never null", got)
	}
}

func TestListConversationsHonorsLimit(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	ingestEvent(t, routes, "wamid.2", "555000111", "John Doe", "1751791100", "hey")
	one := 1

	got := listConversations(t, p, &one)

	if len(got) != 1 || got[0].Contact.Name != "John Doe" {
		t.Fatalf("limited list = %+v, want only the most recent conversation", got)
	}
}

func TestListMessagesReturnsChronologicalThread(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	ingestEvent(t, routes, "wamid.2", "184467235", "María Pérez", "1751791100", "how are you?")
	conversations := listConversations(t, p, nil)
	if len(conversations) != 1 {
		t.Fatalf("conversations = %d, want 1", len(conversations))
	}

	got := listMessages(t, p, conversations[0].ID)

	if len(got) != 2 {
		t.Fatalf("messages = %d, want 2", len(got))
	}
	if got[0].Content != "hello" || got[1].Content != "how are you?" {
		t.Errorf("thread = [%q, %q], want chronological [%q, %q]",
			got[0].Content, got[1].Content, "hello", "how are you?")
	}
	if got[0].Direction != "inbound" || got[0].ContentType != "text" {
		t.Errorf("message = %+v, want inbound text", got[0])
	}
	if got[0].SentAt.Location() != time.UTC {
		t.Errorf("sentAt location = %v, want UTC", got[0].SentAt.Location())
	}
}

func TestListMessagesUnknownConversationIsEmpty(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)

	got := listMessages(t, p, uuid.Must(uuid.NewV7()))

	if len(got) != 0 {
		t.Fatalf("messages = %d, want 0 for an unknown conversation", len(got))
	}
}

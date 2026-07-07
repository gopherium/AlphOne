// SPDX-License-Identifier: AGPL-3.0-or-later

package whatsapp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type conversationBody struct {
	ID             uuid.UUID `json:"id"`
	ContactID      uuid.UUID `json:"contact_id"`
	ContactName    string    `json:"contact_name"`
	ExternalID     string    `json:"external_id"`
	Status         string    `json:"status"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

type messageBody struct {
	ID          uuid.UUID `json:"id"`
	ExternalID  string    `json:"external_id"`
	Direction   string    `json:"direction"`
	Content     string    `json:"content"`
	ContentType string    `json:"content_type"`
	SentAt      time.Time `json:"sent_at"`
}

func getJSON[T any](t *testing.T, routes http.Handler, target string, wantStatus int) T {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("GET %s status = %d, want %d", target, recorder.Code, wantStatus)
	}
	var v T
	if wantStatus == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &v); err != nil {
			t.Fatalf("decoding %q: %v", recorder.Body.String(), err)
		}
	}
	return v
}

func ingestEvent(t *testing.T, routes http.Handler, wamid, waID, name, timestamp, text string) {
	t.Helper()
	body := eventBody(wamid, waID, name, timestamp, text)
	if recorder := postEvent(t, routes, sign("app-secret", body), body); recorder.Code != http.StatusOK {
		t.Fatalf("ingesting %s status = %d, want %d", wamid, recorder.Code, http.StatusOK)
	}
}

func TestListConversationsOrdersByRecentActivity(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")
	ingestEvent(t, routes, "wamid.2", "555000111", "John Doe", "1751791100", "hey")

	got := getJSON[[]conversationBody](t, routes, "/conversations", http.StatusOK)

	if len(got) != 2 {
		t.Fatalf("conversations = %d, want 2", len(got))
	}
	if got[0].ContactName != "John Doe" || got[1].ContactName != "María Pérez" {
		t.Errorf("order = [%q, %q], want most recent first [%q, %q]",
			got[0].ContactName, got[1].ContactName, "John Doe", "María Pérez")
	}
	if got[0].Status != "open" || got[0].ExternalID != "555000111" {
		t.Errorf("conversation = %+v, want status open and external id 555000111", got[0])
	}
	if got[0].LastActivityAt.Location() != time.UTC {
		t.Errorf("last_activity_at location = %v, want UTC", got[0].LastActivityAt.Location())
	}
}

func TestListConversationsEmptyIsAnArray(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)

	request := httptest.NewRequest(http.MethodGet, "/conversations", nil)
	recorder := httptest.NewRecorder()
	p.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != "[]" {
		t.Errorf("body = %q, want %q, never null", body, "[]")
	}
}

func TestListConversationsHonorsLimit(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")
	ingestEvent(t, routes, "wamid.2", "555000111", "John Doe", "1751791100", "hey")

	got := getJSON[[]conversationBody](t, routes, "/conversations?limit=1", http.StatusOK)

	if len(got) != 1 || got[0].ContactName != "John Doe" {
		t.Fatalf("limited list = %+v, want only the most recent conversation", got)
	}
}

func TestListConversationsRejectsBadLimits(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()

	for _, target := range []string{
		"/conversations?limit=abc",
		"/conversations?limit=0",
		"/conversations?limit=1000",
		"/conversations/" + uuid.Must(uuid.NewV7()).String() + "/messages?limit=abc",
	} {
		getJSON[struct{}](t, routes, target, http.StatusBadRequest)
	}
}

func TestListMessagesReturnsChronologicalThread(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")
	ingestEvent(t, routes, "wamid.2", "184467235", "María Pérez", "1751791100", "¿cómo estás?")

	conversations := getJSON[[]conversationBody](t, routes, "/conversations", http.StatusOK)
	if len(conversations) != 1 {
		t.Fatalf("conversations = %d, want 1", len(conversations))
	}

	got := getJSON[[]messageBody](t, routes, "/conversations/"+conversations[0].ID.String()+"/messages", http.StatusOK)

	if len(got) != 2 {
		t.Fatalf("messages = %d, want 2", len(got))
	}
	if got[0].Content != "hola" || got[1].Content != "¿cómo estás?" {
		t.Errorf("thread = [%q, %q], want chronological [%q, %q]",
			got[0].Content, got[1].Content, "hola", "¿cómo estás?")
	}
	if got[0].Direction != "inbound" || got[0].ContentType != "text" {
		t.Errorf("message = %+v, want inbound text", got[0])
	}
	if got[0].SentAt.Location() != time.UTC {
		t.Errorf("sent_at location = %v, want UTC", got[0].SentAt.Location())
	}
}

func TestListMessagesUnknownConversationIsEmpty(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)

	got := getJSON[[]messageBody](t, p.Routes(), "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages", http.StatusOK)

	if len(got) != 0 {
		t.Fatalf("messages = %d, want 0 for an unknown conversation", len(got))
	}
}

func TestListMessagesRejectsMalformedID(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)

	getJSON[struct{}](t, p.Routes(), "/conversations/not-a-uuid/messages", http.StatusBadRequest)
}

func TestReadEndpointsReportStoreFailure(t *testing.T) {
	t.Parallel()

	p := newPlugin(t, unreachableDatabaseURL, nil, nil)
	routes := p.Routes()

	getJSON[struct{}](t, routes, "/conversations", http.StatusInternalServerError)
	getJSON[struct{}](t, routes, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages", http.StatusInternalServerError)
}

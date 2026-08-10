// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

type graphStub struct {
	status   int
	body     string
	lastPath string
	lastAuth string
	lastBody []byte
	server   *httptest.Server
}

func newGraphStub(t *testing.T) *graphStub {
	t.Helper()
	stub := &graphStub{status: http.StatusOK, body: `{"messages":[{"id":"wamid.out.1"}]}`}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.lastPath = r.URL.Path
		stub.lastAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading stub request: %v", err)
		}
		stub.lastBody = body
		w.WriteHeader(stub.status)
		_, _ = w.Write([]byte(stub.body))
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func newSendingHarness(
	t *testing.T, envOverrides map[string]string,
) (*whatsapp.Plugin, *graphStub, *pgxpool.Pool) {
	t.Helper()
	cfg := newTestDatabase(t)
	pool := newAssertionPool(t, cfg.URL())
	resolver := resolverBridge{resolver: contact.NewResolver(postgres.NewContactStore(pool))}
	stub := newGraphStub(t)
	env := map[string]string{
		"ALPHONE_WHATSAPP_APP_SECRET":      "app-secret",
		"ALPHONE_WHATSAPP_ACCESS_TOKEN":    "graph-token",
		"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "555000222",
		"ALPHONE_WHATSAPP_GRAPH_URL":       stub.server.URL,
	}
	for key, value := range envOverrides {
		env[key] = value
	}
	p := newPlugin(t, cfg.URL(), resolver, env)
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p, stub, pool
}

func newSendingPlugin(t *testing.T, envOverrides map[string]string) (*whatsapp.Plugin, *graphStub) {
	t.Helper()
	p, stub, _ := newSendingHarness(t, envOverrides)
	return p, stub
}

// sendMessage sends one reply through the graph resolver.
func sendMessage(
	t *testing.T, p *whatsapp.Plugin, conversationID uuid.UUID, content string,
) (*model.WhatsAppMessage, error) {
	t.Helper()
	return p.MutationResolvers().WhatsAppSendMessage(t.Context(), conversationID, content)
}

// mustSend sends one reply, failing the test on any refusal.
func mustSend(
	t *testing.T, p *whatsapp.Plugin, conversationID uuid.UUID, content string,
) *model.WhatsAppMessage {
	t.Helper()
	sent, err := sendMessage(t, p, conversationID, content)
	if err != nil {
		t.Fatalf("WhatsAppSendMessage() error = %v, want nil", err)
	}
	return sent
}

// sendRefusalCode returns the graph code a refused send carries.
func sendRefusalCode(t *testing.T, err error) string {
	t.Helper()
	var refused sdk.GraphError
	if !errors.As(err, &refused) {
		t.Fatalf("error = %v, want a graph error", err)
	}
	return refused.Code
}

// onlyConversation returns the single conversation the plugin holds.
func onlyConversation(t *testing.T, p *whatsapp.Plugin) *model.WhatsAppConversation {
	t.Helper()
	conversations := listConversations(t, p, nil)
	if len(conversations) != 1 {
		t.Fatalf("conversations = %d, want 1", len(conversations))
	}
	return conversations[0]
}

func TestSendMessageDeliversReply(t *testing.T) {
	t.Parallel()

	p, stub := newSendingPlugin(t, nil)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	conversationID := onlyConversation(t, p).ID

	sent := mustSend(t, p, conversationID, "Ready at 5pm")

	if sent.Direction != "outbound" || sent.Content != "Ready at 5pm" || sent.ExternalID != "wamid.out.1" {
		t.Errorf("message = %+v, want an outbound reply delivered as wamid.out.1", sent)
	}

	if stub.lastPath != "/555000222/messages" {
		t.Errorf("graph path = %q, want %q", stub.lastPath, "/555000222/messages")
	}
	if stub.lastAuth != "Bearer graph-token" {
		t.Errorf("graph authorization = %q, want %q", stub.lastAuth, "Bearer graph-token")
	}
	var payload struct {
		MessagingProduct string `json:"messaging_product"`
		To               string `json:"to"`
		Type             string `json:"type"`
		Text             struct {
			Body string `json:"body"`
		} `json:"text"`
	}
	if err := json.Unmarshal(stub.lastBody, &payload); err != nil {
		t.Fatalf("decoding graph payload %q: %v", stub.lastBody, err)
	}
	if payload.MessagingProduct != "whatsapp" || payload.To != "184467235" ||
		payload.Type != "text" || payload.Text.Body != "Ready at 5pm" {
		t.Errorf("graph payload = %+v, want a whatsapp text to 184467235", payload)
	}

	messages := listMessages(t, p, conversationID)
	if len(messages) != 2 || messages[1].Direction != "outbound" {
		t.Errorf("thread = %+v, want the outbound reply appended after the inbound message", messages)
	}
}

func TestSendMessageAdvancesConversationActivity(t *testing.T) {
	t.Parallel()

	p, _ := newSendingPlugin(t, nil)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	before := onlyConversation(t, p).LastActivityAt

	mustSend(t, p, onlyConversation(t, p).ID, "Ready at 5pm")

	after := onlyConversation(t, p).LastActivityAt
	if !after.After(before) {
		t.Errorf("lastActivityAt = %v, want later than %v after replying", after, before)
	}
}

func TestSendMessageReportsUpstreamFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		configure func(stub *graphStub)
	}{
		"graph error status": {configure: func(stub *graphStub) { stub.status = http.StatusInternalServerError }},
		"unusable response":  {configure: func(stub *graphStub) { stub.body = `{` }},
		"missing message id": {configure: func(stub *graphStub) { stub.body = `{"messages":[]}` }},
		"unreachable graph":  {configure: func(stub *graphStub) { stub.server.Close() }},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p, stub := newSendingPlugin(t, nil)
			routes := p.Routes()
			ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
			conversationID := onlyConversation(t, p).ID
			tc.configure(stub)

			_, err := sendMessage(t, p, conversationID, "hey")

			if code := sendRefusalCode(t, err); code != "UPSTREAM" {
				t.Fatalf("code = %q, want UPSTREAM", code)
			}
			messages := listMessages(t, p, conversationID)
			if len(messages) != 1 {
				t.Errorf("thread = %d messages, want the failed reply not to be stored", len(messages))
			}
		})
	}
}

func TestSendMessageSurfacesGraphErrors(t *testing.T) {
	t.Parallel()

	p, stub := newSendingPlugin(t, nil)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	conversationID := onlyConversation(t, p).ID
	stub.status = http.StatusBadRequest
	stub.body = `{"error":{"message":"Re-engagement message","code":131047}}`

	_, err := sendMessage(t, p, conversationID, "hey")

	var refused sdk.GraphError
	if !errors.As(err, &refused) {
		t.Fatalf("error = %v, want a graph error", err)
	}
	if refused.Extensions["metaCode"] != 131047 {
		t.Fatalf("metaCode = %v, want the rejection code surfaced", refused.Extensions["metaCode"])
	}
	if !strings.Contains(err.Error(), "Re-engagement message") {
		t.Fatalf("error = %v, want the rejection message surfaced", err)
	}
}

func TestSendMessageRejectsMisconfiguredGraphURL(t *testing.T) {
	t.Parallel()

	p, _ := newSendingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_GRAPH_URL": "://not-a-url"})
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	conversationID := onlyConversation(t, p).ID

	_, err := sendMessage(t, p, conversationID, "hey")

	if code := sendRefusalCode(t, err); code != "UPSTREAM" {
		t.Fatalf("code = %q, want UPSTREAM", code)
	}
}

type failingEntropy struct{}

func (failingEntropy) Read([]byte) (int, error) {
	return 0, errors.New("entropy exhausted")
}

func TestSendMessageReportsStoreFailure(t *testing.T) {
	p, _ := newSendingPlugin(t, nil)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	conversationID := onlyConversation(t, p).ID

	uuid.SetRand(failingEntropy{})
	defer uuid.SetRand(nil)

	_, err := sendMessage(t, p, conversationID, "hey")

	if err == nil {
		t.Fatal("WhatsAppSendMessage() error = nil, want the store failure")
	}
}

func TestSendMessageReportsLookupFailure(t *testing.T) {
	t.Parallel()

	p := newPlugin(t, unreachableDatabaseURL, nil, nil)

	_, err := sendMessage(t, p, uuid.Must(uuid.NewV7()), "hey")

	if err == nil {
		t.Fatal("WhatsAppSendMessage() error = nil, want the lookup failure")
	}
}

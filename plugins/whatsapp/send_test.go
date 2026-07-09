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

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/whatsapp"
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

func newSendingPlugin(t *testing.T, envOverrides map[string]string) (*whatsapp.Plugin, *graphStub) {
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
	return p, stub
}

func postJSON(t *testing.T, routes http.Handler, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, request)
	return recorder
}

func onlyConversation(t *testing.T, routes http.Handler) conversationBody {
	t.Helper()
	conversations := getJSON[[]conversationBody](t, routes, "/conversations", http.StatusOK)
	if len(conversations) != 1 {
		t.Fatalf("conversations = %d, want 1", len(conversations))
	}
	return conversations[0]
}

func TestSendMessageBroadcastsToStreamSubscribers(t *testing.T) {
	t.Parallel()

	p, _ := newSendingPlugin(t, nil)
	routes := p.Routes()
	srv := httptest.NewServer(routes)
	t.Cleanup(srv.Close)
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")
	conversationID := onlyConversation(t, routes).ID

	lines := subscribeToEvents(t, srv)
	recorder := postJSON(t, routes, "/conversations/"+conversationID.String()+"/messages", `{"content":"On my way"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	waitForConversationEvent(t, lines)
}

func TestSendMessageDeliversReply(t *testing.T) {
	t.Parallel()

	p, stub := newSendingPlugin(t, nil)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")
	conversationID := onlyConversation(t, routes).ID

	recorder := postJSON(t, routes, "/conversations/"+conversationID.String()+"/messages", `{"content":"Ready at 5pm"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var sent messageBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &sent); err != nil {
		t.Fatalf("decoding %q: %v", recorder.Body.String(), err)
	}
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

	messages := getJSON[[]messageBody](t, routes, "/conversations/"+conversationID.String()+"/messages", http.StatusOK)
	if len(messages) != 2 || messages[1].Direction != "outbound" {
		t.Errorf("thread = %+v, want the outbound reply appended after the inbound message", messages)
	}
}

func TestSendMessageAdvancesConversationActivity(t *testing.T) {
	t.Parallel()

	p, _ := newSendingPlugin(t, nil)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")
	before := onlyConversation(t, routes).LastActivityAt

	recorder := postJSON(
		t,
		routes,
		"/conversations/"+onlyConversation(t, routes).ID.String()+"/messages",
		`{"content":"Ready at 5pm"}`,
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	after := onlyConversation(t, routes).LastActivityAt
	if !after.After(before) {
		t.Errorf("last_activity_at = %v, want later than %v after replying", after, before)
	}
}

func TestSendMessageValidatesRequests(t *testing.T) {
	t.Parallel()

	p, _ := newSendingPlugin(t, nil)
	routes := p.Routes()

	tests := map[string]struct {
		target     string
		body       string
		wantStatus int
	}{
		"malformed conversation id": {
			target:     "/conversations/not-a-uuid/messages",
			body:       `{"content":"hey"}`,
			wantStatus: http.StatusBadRequest,
		},
		"malformed body": {
			target:     "/conversations/" + uuid.Must(uuid.NewV7()).String() + "/messages",
			body:       `{"content":`,
			wantStatus: http.StatusBadRequest,
		},
		"blank content": {
			target:     "/conversations/" + uuid.Must(uuid.NewV7()).String() + "/messages",
			body:       `{"content":" \t "}`,
			wantStatus: http.StatusBadRequest,
		},
		"unknown conversation": {
			target:     "/conversations/" + uuid.Must(uuid.NewV7()).String() + "/messages",
			body:       `{"content":"hey"}`,
			wantStatus: http.StatusNotFound,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			recorder := postJSON(t, routes, tc.target, tc.body)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
		})
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
			ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")
			conversationID := onlyConversation(t, routes).ID
			tc.configure(stub)

			recorder := postJSON(t, routes, "/conversations/"+conversationID.String()+"/messages", `{"content":"hey"}`)

			if recorder.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
			}
			messages := getJSON[[]messageBody](t, routes, "/conversations/"+conversationID.String()+"/messages", http.StatusOK)
			if len(messages) != 1 {
				t.Errorf("thread = %d messages, want the failed reply not to be stored", len(messages))
			}
		})
	}
}

func TestSendMessageRejectsMisconfiguredGraphURL(t *testing.T) {
	t.Parallel()

	p, _ := newSendingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_GRAPH_URL": "://not-a-url"})
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")
	conversationID := onlyConversation(t, routes).ID

	recorder := postJSON(t, routes, "/conversations/"+conversationID.String()+"/messages", `{"content":"hey"}`)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
	}
}

type failingEntropy struct{}

func (failingEntropy) Read([]byte) (int, error) {
	return 0, errors.New("entropy exhausted")
}

func TestSendMessageReportsStoreFailure(t *testing.T) {
	p, _ := newSendingPlugin(t, nil)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")
	conversationID := onlyConversation(t, routes).ID

	uuid.SetRand(failingEntropy{})
	defer uuid.SetRand(nil)

	recorder := postJSON(t, routes, "/conversations/"+conversationID.String()+"/messages", `{"content":"hey"}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestSendMessageReportsLookupFailure(t *testing.T) {
	t.Parallel()

	p := newPlugin(t, unreachableDatabaseURL, nil, nil)
	routes := p.Routes()

	recorder := postJSON(t, routes, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages", `{"content":"hey"}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

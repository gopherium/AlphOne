// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// newGraphStub starts a stub graph server that records each call and answers a canned response.
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

// newSendingHarness returns a migrated plugin wired to a stub graph server, together with that stub and the pool.
func newSendingHarness(
	t *testing.T, envOverrides map[string]string,
) (*whatsapp.Plugin, *graphStub, *pgxpool.Pool) {
	t.Helper()
	address := newTestDatabase(t)
	pool := newAssertionPool(t, address)
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
	p := newPlugin(t, address, resolver, env)
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p, stub, pool
}

// newSendingPlugin returns the migrated plugin and stub graph server of a sending harness.
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

func TestSendMessageNamesTheReasonForEmptyContent(t *testing.T) {
	t.Parallel()

	p, _ := newSendingPlugin(t, nil)

	_, err := sendMessage(t, p, uuid.Must(uuid.NewV7()), "")

	var raised sdk.GraphError
	if !errors.As(err, &raised) || raised.Reason != "message_content_required" {
		t.Errorf("error = %v, want the empty content named as a reason", err)
	}
}

// mustSend sends one reply, failing the test on any error.
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

// sendErrorCode returns the graph code a failed send carries.
func sendErrorCode(t *testing.T, err error) string {
	t.Helper()
	var refused sdk.GraphError
	if !errors.As(err, &refused) {
		t.Fatalf("error = %v, want a graph error", err)
	}
	return refused.Code
}

// sendFailure is the code and the reason a failed send carries.
type sendFailure struct {
	code   string
	reason string
}

// sendFailureOf returns what a graph error carries, nothing for any other answer.
func sendFailureOf(err error) sendFailure {
	var refused sdk.GraphError
	if !errors.As(err, &refused) {
		return sendFailure{}
	}
	return sendFailure{code: refused.Code, reason: refused.Reason}
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

// ingestUserIDEvent posts one signed text message whose sender carries a user ID and no phone number.
func ingestUserIDEvent(t *testing.T, routes http.Handler, wamid, userID string) {
	t.Helper()
	body := fmt.Appendf(nil, `{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"contacts": [{"user_id": %q, "profile": {"name": "Maria Perez"}}],
			"messages": [{"from_user_id": %q, "id": %q, "timestamp": "1751791000", "type": "text",
				"text": {"body": "hello"}}]
		}}]}]
	}`, userID, userID, wamid)
	if recorder := postEvent(t, routes, sign("app-secret", body), body); recorder.Code != http.StatusOK {
		t.Fatalf("ingesting %s status = %d, want %d", wamid, recorder.Code, http.StatusOK)
	}
}

// sentFields returns the fields of the last request the stub graph server received.
func sentFields(t *testing.T, stub *graphStub) map[string]any {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal(stub.lastBody, &fields); err != nil {
		t.Fatalf("decoding graph payload %q: %v", stub.lastBody, err)
	}
	return fields
}

// wantAddressed fails unless fields names address in field and leaves the other field out.
func wantAddressed(t *testing.T, fields map[string]any, field, other, address string) {
	t.Helper()
	if fields[field] != address {
		t.Errorf("graph payload %s = %v, want %s", field, fields[field], address)
	}
	if left, named := fields[other]; named {
		t.Errorf("graph payload also names %s = %v, want no %s", other, left, other)
	}
}

func TestSendMessageUsesRecipientAndNoToForAUserIDAddress(t *testing.T) {
	t.Parallel()

	p, stub := newSendingPlugin(t, nil)
	ingestUserIDEvent(t, p.Routes(), "wamid.u", "US.184467235")

	mustSend(t, p, onlyConversation(t, p).ID, "Ready at 5pm")

	wantAddressed(t, sentFields(t, stub), "recipient", "to", "US.184467235")
}

func TestSendMessageUsesToAndNoRecipientForAPhoneAddress(t *testing.T) {
	t.Parallel()

	p, stub := newSendingPlugin(t, nil)
	ingestEvent(t, p.Routes(), "wamid.p", "184467235", "Maria Perez", "1751791000", "hello")

	mustSend(t, p, onlyConversation(t, p).ID, "Ready at 5pm")

	wantAddressed(t, sentFields(t, stub), "to", "recipient", "184467235")
}

func TestSendMessageUsesToForALidAddress(t *testing.T) {
	t.Parallel()

	p, stub := newSendingPlugin(t, nil)
	ingestEvent(t, p.Routes(), "wamid.l", "184467235@lid", "Maria Perez", "1751791000", "hello")

	mustSend(t, p, onlyConversation(t, p).ID, "Ready at 5pm")

	wantAddressed(t, sentFields(t, stub), "to", "recipient", "184467235@lid")
}

func TestSendMessageUsesTheCallersTenantCredentials(t *testing.T) {
	t.Parallel()

	p, stub, pool := newSendingHarness(t, nil)
	p.UseEncrypter(whatsapp.BoundEncrypter())
	acme := seedTenant(t, pool)
	mine := sdk.WithTenant(t.Context(), acme)
	stored := sdk.ChannelCredentials{Identifier: "555000333", Secret: "EAAG-acme-token"}
	if err := p.SetChannelCredentials(mine, stored); err != nil {
		t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
	}
	body := numberedEventBody("555000333", "wamid.acme")
	if recorder := postEvent(t, p.Routes(), sign("app-secret", body), body); recorder.Code != http.StatusOK {
		t.Fatalf("ingest status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if _, err := p.MutationResolvers().WhatsAppSendMessage(mine, onlyConversationID(t, pool), "Ready at 5pm"); err != nil {
		t.Fatalf("WhatsAppSendMessage() error = %v, want nil", err)
	}

	if stub.lastPath != "/555000333/messages" {
		t.Errorf("graph path = %q, want the tenant's number %q", stub.lastPath, "/555000333/messages")
	}
	if stub.lastAuth != "Bearer EAAG-acme-token" {
		t.Errorf("graph authorization = %q, want the tenant's token", stub.lastAuth)
	}
}

// conversationOn returns the conversation stored on a number.
func conversationOn(t *testing.T, pool *pgxpool.Pool, number string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(t.Context(),
		`SELECT id FROM plugin_whatsapp.conversations WHERE phone_number_id = $1`, number,
	).Scan(&id); err != nil {
		t.Fatalf("reading the conversation on %s: %v", number, err)
	}
	return id
}

// newTwoNumberTenant returns a sending plugin whose tenant holds two numbers a customer wrote to, beside its context.
func newTwoNumberTenant(t *testing.T) (*whatsapp.Plugin, *graphStub, *pgxpool.Pool, context.Context) {
	t.Helper()
	p, stub, pool := newSendingHarness(t, nil)
	p.UseEncrypter(whatsapp.BoundEncrypter())
	acme := seedTenant(t, pool)
	for _, number := range []string{"5550001", "5550002"} {
		connectNumber(t, p, acme, numberWithSecret(number, "secret-"+number))
		body := numberedEventBody(number, "wamid."+number)
		if recorder := postEvent(t, p.Routes(), sign("secret-"+number, body), body); recorder.Code != http.StatusOK {
			t.Fatalf("ingest status = %d, want %d", recorder.Code, http.StatusOK)
		}
	}
	return p, stub, pool, sdk.WithTenant(t.Context(), acme)
}

func TestAReplyLeavesFromTheNumberItsConversationCameInOn(t *testing.T) {
	t.Parallel()

	p, stub, pool, mine := newTwoNumberTenant(t)

	for _, number := range []string{"5550001", "5550002"} {
		stub.body = `{"messages":[{"id":"wamid.out.` + number + `"}]}`
		_, err := p.MutationResolvers().WhatsAppSendMessage(mine, conversationOn(t, pool, number), "Ready at 5pm")
		if err != nil {
			t.Fatalf("WhatsAppSendMessage() error = %v, want nil", err)
		}
		if stub.lastPath != "/"+number+"/messages" {
			t.Errorf("graph path = %q, want the number the conversation came in on", stub.lastPath)
		}
		if stub.lastAuth != "Bearer EAAG-"+number {
			t.Errorf("graph authorization = %q, want the token of %s", stub.lastAuth, number)
		}
	}
}

func TestAReplyOnARemovedNumberNamesTheReasonAndSendsNothing(t *testing.T) {
	t.Parallel()

	p, stub, pool, mine := newTwoNumberTenant(t)
	if err := p.RemoveChannelCredentials(mine, "5550001"); err != nil {
		t.Fatalf("RemoveChannelCredentials() error = %v, want nil", err)
	}

	_, err := p.MutationResolvers().WhatsAppSendMessage(mine, conversationOn(t, pool, "5550001"), "Ready at 5pm")

	if got := sendFailureOf(err); got != (sendFailure{code: "VALIDATION", reason: "number_removed"}) {
		t.Errorf("error = %v, want the reason number_removed", err)
	}
	if stub.lastAuth != "" {
		t.Errorf("graph authorization = %q, want no call on a removed number", stub.lastAuth)
	}
}

func TestAConversationOnARemovedNumberStaysReadable(t *testing.T) {
	t.Parallel()

	p, _, pool, mine := newTwoNumberTenant(t)
	conversationID := conversationOn(t, pool, "5550001")
	if err := p.RemoveChannelCredentials(mine, "5550001"); err != nil {
		t.Fatalf("RemoveChannelCredentials() error = %v, want nil", err)
	}

	read, err := p.QueryResolvers().WhatsAppConversation(mine, conversationID)
	if err != nil {
		t.Fatalf("WhatsAppConversation() error = %v, want nil", err)
	}
	if read == nil {
		t.Fatal("WhatsAppConversation() = nil, want the conversation kept")
	}
	messages, err := p.WhatsAppConversationResolvers().Messages(mine, read, nil, nil)
	if err != nil {
		t.Fatalf("Messages() error = %v, want nil", err)
	}
	if len(messages) != 1 {
		t.Errorf("messages = %d, want the message kept", len(messages))
	}
}

func TestANumberAnotherTenantStoredStaysRemovedForTheOneThatLeftIt(t *testing.T) {
	t.Parallel()

	p, stub, pool, mine := newTwoNumberTenant(t)
	if err := p.RemoveChannelCredentials(mine, "5550001"); err != nil {
		t.Fatalf("RemoveChannelCredentials() error = %v, want nil", err)
	}
	connectNumber(t, p, seedGlobex(t, pool), numberWithSecret("5550001", "globex-secret"))

	_, err := p.MutationResolvers().WhatsAppSendMessage(mine, conversationOn(t, pool, "5550001"), "Ready at 5pm")

	var refused sdk.GraphError
	if !errors.As(err, &refused) || refused.Reason != "number_removed" {
		t.Errorf("error = %v, want the reason number_removed", err)
	}
	if stub.lastAuth != "" {
		t.Errorf("graph authorization = %q, want no call with the token of the new holder", stub.lastAuth)
	}
}

func TestAReplyOnAnEnvironmentNumberWithoutATokenSendsNothing(t *testing.T) {
	t.Parallel()

	p, stub, _ := newSendingHarness(t, map[string]string{"ALPHONE_WHATSAPP_ACCESS_TOKEN": ""})
	p.UseEncrypter(whatsapp.BoundEncrypter())
	connectNumber(t, p, sdk.DefaultTenantID, numberWithSecret("5550001", "secret-5550001"))
	ingestEvent(t, p.Routes(), "wamid.env", "184467235", "Maria Perez", "1751791000", "hello")

	_, err := sendMessage(t, p, onlyConversation(t, p).ID, "Ready at 5pm")

	var refused sdk.GraphError
	if !errors.As(err, &refused) || refused.Reason != "credentials_missing" {
		t.Errorf("error = %v, want the missing credentials named as a reason", err)
	}
	if stub.lastAuth != "" {
		t.Errorf("graph authorization = %q, want no call from a number the customer never wrote to", stub.lastAuth)
	}
}

func TestAConversationFromBeforeRepliesFromTheOldestNumber(t *testing.T) {
	t.Parallel()

	p, stub, pool, mine := newTwoNumberTenant(t)
	conversationID := conversationOn(t, pool, "5550002")
	if _, err := pool.Exec(t.Context(),
		`UPDATE plugin_whatsapp.conversations SET phone_number_id = NULL WHERE id = $1`, conversationID); err != nil {
		t.Fatalf("clearing the number: %v", err)
	}

	if _, err := p.MutationResolvers().WhatsAppSendMessage(mine, conversationID, "Ready at 5pm"); err != nil {
		t.Fatalf("WhatsAppSendMessage() error = %v, want nil", err)
	}

	if stub.lastPath != "/5550001/messages" {
		t.Errorf("graph path = %q, want the tenant's oldest number", stub.lastPath)
	}
}

func TestAnOlderConversationRepliesWithTheStoredTokenOnceTheEnvironmentNumberIsConnected(t *testing.T) {
	t.Parallel()

	p, stub := newSendingPlugin(t, nil)
	p.UseEncrypter(whatsapp.BoundEncrypter())
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.older", "184467235", "Maria Perez", "1751791000", "hello")
	older := onlyConversation(t, p).ID
	connectNumber(t, p, sdk.DefaultTenantID, numberWithSecret("555000222", "secret-555000222"))
	body := numberedEventBody("555000222", "wamid.newer")
	if recorder := postEvent(t, routes, sign("secret-555000222", body), body); recorder.Code != http.StatusOK {
		t.Fatalf("ingest status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := len(listConversations(t, p, nil)); held != 2 {
		t.Fatalf("conversations = %d, want the customer's next message on a conversation of its own", held)
	}

	mustSend(t, p, older, "Ready at 5pm")

	if stub.lastPath != "/555000222/messages" {
		t.Errorf("graph path = %q, want the environment number", stub.lastPath)
	}
	if stub.lastAuth != "Bearer EAAG-555000222" {
		t.Errorf("graph authorization = %q, want the token of the stored row", stub.lastAuth)
	}
}

func TestSendMessageRefusesATenantWithoutCredentials(t *testing.T) {
	t.Parallel()

	p, stub, pool := newSendingHarness(t, nil)
	p.UseEncrypter(whatsapp.BoundEncrypter())
	acme := seedTenant(t, pool)
	seedOutboundRow(t, pool, "wamid.bare", acme)

	_, err := p.MutationResolvers().WhatsAppSendMessage(
		sdk.WithTenant(t.Context(), acme), onlyConversationID(t, pool), "hey",
	)

	var refused sdk.GraphError
	if !errors.As(err, &refused) || refused.Reason != "credentials_missing" {
		t.Errorf("error = %v, want the missing credentials named as a reason", err)
	}
	if stub.lastAuth != "" {
		t.Errorf("graph authorization = %q, want no call without credentials", stub.lastAuth)
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

			if code := sendErrorCode(t, err); code != "UPSTREAM" {
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

	if code := sendErrorCode(t, err); code != "UPSTREAM" {
		t.Fatalf("code = %q, want UPSTREAM", code)
	}
}

type failingEntropy struct{}

// Read returns an entropy failure instead of random bytes.
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

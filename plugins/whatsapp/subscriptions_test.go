// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// subscriptionPatience bounds how long a test waits on a subscription.
const subscriptionPatience = 20 * time.Second

// subscribingServer returns a running graph server over the plugin's composed root.
func subscribingServer(t *testing.T, p *whatsapp.Plugin, pool *pgxpool.Pool) *httptest.Server {
	t.Helper()
	graph := newGraphQLServer(t, p, pool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graph.ServeHTTP(w, r.WithContext(sdk.WithRequestScope(r.Context(), sdk.NewRequestScope())))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// openSubscription posts a subscription to the graph over the event stream transport.
func openSubscription(t *testing.T, srv *httptest.Server, query string) *bufio.Reader {
	t.Helper()
	body := fmt.Sprintf(`{"query":%q}`, query)
	ctx, cancel := context.WithTimeout(t.Context(), subscriptionPatience)
	t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the subscription request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	response, err := srv.Client().Do(request)
	if err != nil {
		t.Fatalf("opening the subscription: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("subscription Content-Type = %q, want the event stream transport to serve it", got)
	}
	return bufio.NewReader(response.Body)
}

// nextData returns the next data frame written to a subscription stream.
func nextData(t *testing.T, frames *bufio.Reader) string {
	t.Helper()
	var seen strings.Builder
	for {
		line, err := frames.ReadString('\n')
		if err != nil {
			t.Fatalf("reading a subscription frame: %v, after %q", err, seen.String())
		}
		seen.WriteString(line)
		if strings.HasPrefix(line, "data: ") {
			return line
		}
	}
}

// scanData sends the stream's next data frame, if one arrives at all.
func scanData(frames *bufio.Reader, arrived chan<- string) {
	for {
		line, err := frames.ReadString('\n')
		if err != nil {
			return
		}
		if strings.HasPrefix(line, "data: ") {
			arrived <- line
			return
		}
	}
}

// noDataWithin fails when a data frame arrives on the stream before wait ends.
func noDataWithin(t *testing.T, frames *bufio.Reader, wait time.Duration) {
	t.Helper()
	arrived := make(chan string, 1)
	go scanData(frames, arrived)
	select {
	case line := <-arrived:
		t.Errorf("a quiet stream read %q, want nothing", line)
	case <-time.After(wait):
	}
}

// deliverMessage posts a signed inbound webhook carrying one text message.
func deliverMessage(t *testing.T, p *whatsapp.Plugin, wamid, text string) {
	t.Helper()
	body := eventBody(wamid, "184467235", "Maria Perez", "1753900000", text)
	if recorder := postEvent(t, p.Routes(), sign("app-secret", body), body); recorder.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body)
	}
}

// waitForListeners blocks until the plugin's broadcaster holds count listeners.
func waitForListeners(t *testing.T, p *whatsapp.Plugin, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p.Subscribers() == count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("broadcaster holds %d listeners, want %d", p.Subscribers(), count)
}

// onlyConversationID returns the id of the single stored conversation.
func onlyConversationID(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(t.Context(),
		`SELECT id FROM plugin_whatsapp.conversations`,
	).Scan(&id); err != nil {
		t.Fatalf("reading the conversation: %v", err)
	}
	return id
}

func TestConversationEventSubscriptionDeliversAChangedConversation(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	srv := subscribingServer(t, p, pool)
	deliverMessage(t, p, "wamid.first", "Hello there")
	conversationID := onlyConversationID(t, pool)
	frames := openSubscription(t, srv, "subscription { whatsAppConversationEvent }")
	waitForListeners(t, p, 1)

	deliverMessage(t, p, "wamid.second", "Are you there")

	if got := nextData(t, frames); !strings.Contains(got, conversationID.String()) {
		t.Errorf("subscription read %q, want the changed conversation %s", got, conversationID)
	}
}

func TestMessageSubscriptionDeliversTheArrival(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	srv := subscribingServer(t, p, pool)
	deliverMessage(t, p, "wamid.first", "Hello there")
	conversationID := onlyConversationID(t, pool)
	frames := openSubscription(t, srv, fmt.Sprintf(
		`subscription { whatsAppMessageReceived(conversationId: %q) { externalId content direction } }`,
		conversationID,
	))
	waitForListeners(t, p, 1)

	deliverMessage(t, p, "wamid.second", "Are you there")

	got := nextData(t, frames)
	for _, want := range []string{"wamid.second", "Are you there", "inbound"} {
		if !strings.Contains(got, want) {
			t.Errorf("subscription read %q, want it to carry %q", got, want)
		}
	}
}

func TestMessageSubscriptionIgnoresAnotherConversation(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	srv := subscribingServer(t, p, pool)
	deliverMessage(t, p, "wamid.first", "Hello there")
	conversationID := onlyConversationID(t, pool)
	elsewhere := openSubscription(t, srv, fmt.Sprintf(
		`subscription { whatsAppMessageReceived(conversationId: %q) { content } }`, uuid.Must(uuid.NewV7()),
	))
	watched := openSubscription(t, srv, fmt.Sprintf(
		`subscription { whatsAppMessageReceived(conversationId: %q) { content } }`, conversationID,
	))
	waitForListeners(t, p, 2)

	deliverMessage(t, p, "wamid.second", "Are you there")

	if got := nextData(t, watched); !strings.Contains(got, "Are you there") {
		t.Errorf("the watched thread read %q, want the arrival", got)
	}
	noDataWithin(t, elsewhere, 300*time.Millisecond)
}

func TestConversationEventSubscriptionDeliversAnOutboundSend(t *testing.T) {
	t.Parallel()

	p, _, pool := newSendingHarness(t, nil)
	routes := p.Routes()
	srv := subscribingServer(t, p, pool)
	ingestEvent(t, routes, "wamid.1", "184467235", "Maria Perez", "1751791000", "hello")
	conversationID := onlyConversationID(t, pool)
	frames := openSubscription(t, srv, "subscription { whatsAppConversationEvent }")
	waitForListeners(t, p, 1)

	mustSend(t, p, conversationID, "On my way")

	if got := nextData(t, frames); !strings.Contains(got, conversationID.String()) {
		t.Errorf("subscription read %q, want the changed conversation %s", got, conversationID)
	}
}

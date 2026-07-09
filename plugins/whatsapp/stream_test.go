// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func subscribeToEvents(t *testing.T, srv *httptest.Server) chan string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("connecting to stream: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	lines := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if payload, ok := strings.CutPrefix(line, "data: "); ok {
				lines <- strings.TrimSpace(payload)
				return
			}
		}
	}()
	return lines
}

func waitForConversationEvent(t *testing.T, lines chan string) {
	t.Helper()
	select {
	case got := <-lines:
		if !strings.Contains(got, "conversation") {
			t.Errorf("event data = %q, want a conversation change event", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no SSE event after the message activity")
	}
}

func TestWebhookEventBroadcastsToStreamSubscribers(t *testing.T) {
	t.Parallel()

	p, _ := newIngestingPlugin(t)
	routes := p.Routes()
	srv := httptest.NewServer(routes)
	t.Cleanup(srv.Close)

	lines := subscribeToEvents(t, srv)

	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hola")

	waitForConversationEvent(t, lines)
}

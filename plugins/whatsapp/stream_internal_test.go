// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var errBoom = errors.New("boom")

type noFlushWriter struct {
	header http.Header
	status int
}

func (n *noFlushWriter) Header() http.Header         { return n.header }
func (n *noFlushWriter) Write(b []byte) (int, error) { return len(b), nil }
func (n *noFlushWriter) WriteHeader(status int)      { n.status = status }

type fakeStreamWriter struct {
	header   http.Header
	writeErr error
}

func (f *fakeStreamWriter) Header() http.Header { return f.header }
func (f *fakeStreamWriter) Write(b []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(b), nil
}
func (f *fakeStreamWriter) WriteHeader(int) {}
func (f *fakeStreamWriter) Flush()          {}

func (b *broadcaster) subscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within 2s")
}

func TestStreamDeliversEventsThenCleansUp(t *testing.T) {
	t.Parallel()

	p := &Plugin{events: newBroadcaster()}
	router := chi.NewRouter()
	router.Get("/events", p.handleStream())
	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("connecting to stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	waitFor(t, func() bool { return p.events.subscriberCount() == 1 })

	want := uuid.Must(uuid.NewV7())
	p.events.broadcast(event{Conversation: want})

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

	select {
	case got := <-lines:
		if !strings.Contains(got, want.String()) {
			t.Errorf("event data = %q, want it to contain %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no SSE event received within 2s")
	}

	cancel()
	waitFor(t, func() bool { return p.events.subscriberCount() == 0 })
}

func TestStreamClosesAtItsLifetime(t *testing.T) {
	t.Parallel()

	p := &Plugin{events: newBroadcaster(), streamLifetime: 20 * time.Millisecond}
	w := &fakeStreamWriter{header: http.Header{}}
	start := time.Now()
	done := make(chan struct{})
	go func() {
		p.handleStream()(w, httptest.NewRequest(http.MethodGet, "/events", nil))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not close at its configured lifetime")
	}
	if elapsed := time.Since(start); elapsed < p.streamLifetime {
		t.Errorf("stream closed after %v, want it to stay open at least %v", elapsed, p.streamLifetime)
	}
	waitFor(t, func() bool { return p.events.subscriberCount() == 0 })
}

func TestStreamStaysOpenWithoutALifetime(t *testing.T) {
	t.Parallel()

	p := &Plugin{events: newBroadcaster(), streamLifetime: 0}
	w := &fakeStreamWriter{header: http.Header{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		p.handleStream()(w, req)
		close(done)
	}()

	waitFor(t, func() bool { return p.events.subscriberCount() == 1 })

	select {
	case <-done:
		t.Fatal("stream closed without a lifetime or a cancelled request")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not close when the request was cancelled")
	}
}

func TestStreamRejectsAnUnflushableWriter(t *testing.T) {
	t.Parallel()

	p := &Plugin{events: newBroadcaster()}
	w := &noFlushWriter{header: http.Header{}}

	p.handleStream()(w, httptest.NewRequest(http.MethodGet, "/events", nil))

	if w.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.status, http.StatusInternalServerError)
	}
}

func TestStreamStopsWhenTheClientDisconnects(t *testing.T) {
	t.Parallel()

	p := &Plugin{events: newBroadcaster()}
	w := &fakeStreamWriter{header: http.Header{}, writeErr: errBoom}
	done := make(chan struct{})
	go func() {
		p.handleStream()(w, httptest.NewRequest(http.MethodGet, "/events", nil))
		close(done)
	}()

	waitFor(t, func() bool { return p.events.subscriberCount() == 1 })
	p.events.broadcast(event{Conversation: uuid.Must(uuid.NewV7())})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after a failed write")
	}
}

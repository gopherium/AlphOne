// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/event"
)

var errStreamBoom = errors.New("boom")

type noFlushWriter struct {
	header http.Header
	status int
}

func (n *noFlushWriter) Header() http.Header         { return n.header }
func (n *noFlushWriter) Write(b []byte) (int, error) { return len(b), nil }
func (n *noFlushWriter) WriteHeader(status int)      { n.status = status }

type failingStreamWriter struct {
	header   http.Header
	writeErr error
	wrote    bool
}

func (f *failingStreamWriter) Header() http.Header { return f.header }
func (f *failingStreamWriter) Write(b []byte) (int, error) {
	f.wrote = true
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(b), nil
}
func (f *failingStreamWriter) WriteHeader(int) {}
func (f *failingStreamWriter) Flush()          {}

type gatedFlushWriter struct {
	header  http.Header
	release chan struct{}
	mu      sync.Mutex
	buf     strings.Builder
}

func (g *gatedFlushWriter) Header() http.Header { return g.header }
func (g *gatedFlushWriter) Write(b []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.Write(b)
}
func (g *gatedFlushWriter) WriteHeader(int) {}
func (g *gatedFlushWriter) Flush()          { <-g.release }

func (g *gatedFlushWriter) written() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.String()
}

func waitForCondition(t *testing.T, cond func() bool) {
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

func TestEventStreamAnswers503WithoutAHub(t *testing.T) {
	t.Parallel()

	s := &server{}
	recorder := httptest.NewRecorder()

	s.handleEventStream()(recorder, httptest.NewRequest(http.MethodGet, "/api/events", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestEventStreamRejectsAWriterWithoutFlush(t *testing.T) {
	t.Parallel()

	s := &server{live: event.NewHub()}
	w := &noFlushWriter{header: http.Header{}}

	s.handleEventStream()(w, httptest.NewRequest(http.MethodGet, "/api/events", nil))

	if w.status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.status, http.StatusInternalServerError)
	}
	if s.live.Subscribers() != 0 {
		t.Error("subscription left behind, want it removed")
	}
}

func TestEventStreamStopsWhenAWriteFails(t *testing.T) {
	t.Parallel()

	s := &server{live: event.NewHub()}
	w := &failingStreamWriter{header: http.Header{}, writeErr: errStreamBoom}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})

	go func() {
		defer close(done)
		s.handleEventStream()(w, httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx))
	}()
	waitForCondition(t, func() bool { return s.live.Subscribers() == 1 })
	s.live.Broadcast(event.Frame{Name: event.TaskCreated})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler kept running after a write failure")
	}
	if !w.wrote {
		t.Error("nothing was written, want the failing write attempted")
	}
}

func TestDrainNamesStopsWhenAWriteFails(t *testing.T) {
	t.Parallel()

	w := &failingStreamWriter{header: http.Header{}, writeErr: errStreamBoom}
	subscription := make(chan event.Name, 2)
	subscription <- event.TaskCreated
	subscription <- event.TaskCreated

	drainNames(w, http.NewResponseController(w), subscription)

	if len(subscription) != 1 {
		t.Errorf("%d names left buffered, want the drain stopped after the failing write", len(subscription))
	}
}

func TestEventStreamWritesBufferedNamesBeforeClosing(t *testing.T) {
	t.Parallel()

	s := &server{live: event.NewHub()}
	w := &gatedFlushWriter{header: http.Header{}, release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		s.handleEventStream()(w, httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx))
	}()
	waitForCondition(t, func() bool { return s.live.Subscribers() == 1 })
	for range 3 {
		s.live.Broadcast(event.Frame{Name: event.TaskCreated})
	}
	cancel()
	close(w.release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not finish after cancellation")
	}
	if got := strings.Count(w.written(), "data: "); got != 3 {
		t.Errorf("wrote %d frames, want the 3 buffered names drained", got)
	}
}

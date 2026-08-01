// SPDX-License-Identifier: AGPL-3.0-or-later

package server_test

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/server"
)

// liveServer returns a running test server with a live hub, and a logged in cookie.
func liveServer(t *testing.T, hub *event.Hub, lifetime time.Duration, perUser int) (*httptest.Server, *http.Cookie) {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	handler := server.NewServer(server.Config{
		Contacts:          newFakeContactStore(),
		Users:             users,
		Live:              hub,
		MaxStreamLifetime: lifetime,
		MaxStreamsPerUser: perUser,
	})
	cookie := loginCookie(t, handler)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, cookie
}

func TestEventStreamDeliversNamesAndClosesAtItsLifetime(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	srv, cookie := liveServer(t, hub, 1500*time.Millisecond, 5)
	request, err := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	request.AddCookie(cookie)

	response, err := srv.Client().Do(request)
	if err != nil {
		t.Fatalf("connecting to stream: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if ct := response.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	for range 200 {
		if hub.Subscribers() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	hub.Broadcast(event.TaskCreated)

	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("reading the first frame: %v", err)
	}
	if !strings.Contains(line, "task.created") {
		t.Errorf("frame = %q, want it to carry task.created", line)
	}

	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("draining the stream: %v", err)
	}
	if hub.Subscribers() != 0 {
		t.Error("subscription survived the lifetime, want it removed")
	}
}

func TestEventStreamCapsConcurrentStreamsPerUser(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	srv, cookie := liveServer(t, hub, time.Minute, 1)
	first, err := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	first.AddCookie(cookie)
	held, err := srv.Client().Do(first)
	if err != nil {
		t.Fatalf("opening the first stream: %v", err)
	}
	defer func() { _ = held.Body.Close() }()

	second, err := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	second.AddCookie(cookie)
	rejected, err := srv.Client().Do(second)
	if err != nil {
		t.Fatalf("opening the second stream: %v", err)
	}
	defer func() { _ = rejected.Body.Close() }()

	if rejected.StatusCode != http.StatusTooManyRequests {
		t.Errorf("second stream status = %d, want %d", rejected.StatusCode, http.StatusTooManyRequests)
	}
}

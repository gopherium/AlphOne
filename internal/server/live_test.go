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

	"github.com/gopherium/gouncer"

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

// twoUserLiveServer returns a running test server, the first user, and a cookie each.
func twoUserLiveServer(t *testing.T, hub *event.Hub) (*httptest.Server, gouncer.User, [2]*http.Cookie) {
	t.Helper()
	users := newFakeUserStore()
	ada := addAda(t, users)
	users.AddUser(t, "grace@example.com", "Grace Hopper", testPassword)
	handler := server.NewServer(server.Config{
		Contacts:          newFakeContactStore(),
		Users:             users,
		Live:              hub,
		MaxStreamLifetime: time.Minute,
		MaxStreamsPerUser: 5,
	})
	second := doLogin(t, handler, `{"email":"grace@example.com","password":"`+testPassword+`"}`)
	if second.Code != http.StatusOK {
		t.Fatalf("second login status = %d, want %d", second.Code, http.StatusOK)
	}
	cookies := [2]*http.Cookie{loginCookie(t, handler), sessionCookie(t, second)}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, ada, cookies
}

// openStream connects to the event stream and returns a reader over its frames.
func openStream(t *testing.T, srv *httptest.Server, cookie *http.Cookie) *bufio.Reader {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	request.AddCookie(cookie)
	response, err := srv.Client().Do(request)
	if err != nil {
		t.Fatalf("connecting to stream: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return bufio.NewReader(response.Body)
}

// nextFrame returns the next data frame written to the stream.
func nextFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("reading a frame: %v", err)
	}
	return line
}

func TestEventStreamDeliversATargetedNameOnlyToItsAudience(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	srv, assignee, cookies := twoUserLiveServer(t, hub)
	assigneeStream := openStream(t, srv, cookies[0])
	bystanderStream := openStream(t, srv, cookies[1])
	for range 200 {
		if hub.Subscribers() == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	hub.Broadcast(event.Frame{Name: event.TaskCreated, Audience: assignee.ID})
	hub.Broadcast(event.Frame{Name: event.ContactCreated})

	if got := nextFrame(t, assigneeStream); !strings.Contains(got, "task.created") {
		t.Errorf("the assignee read %q, want it to carry task.created", got)
	}
	if got := nextFrame(t, bystanderStream); !strings.Contains(got, "contact.created") {
		t.Errorf("the bystander read %q, want the targeted name withheld", got)
	}
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
	hub.Broadcast(event.Frame{Name: event.TaskCreated})

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

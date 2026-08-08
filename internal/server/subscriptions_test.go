// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"bufio"
	"context"
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

// subscriptionPatience bounds how long a test waits on a subscription.
const subscriptionPatience = 20 * time.Second

// coreEventSubscription subscribes to the core event names.
const coreEventSubscription = `{"query":"subscription { coreEvent }"}`

// subscriptionSlots is the per user stream budget a subscribing server grants.
const subscriptionSlots = 5

// subscribingServer returns a running graph server with a live hub, the first user, and a cookie each.
func subscribingServer(
	t *testing.T, hub *event.Hub, lifetime time.Duration,
) (*httptest.Server, gouncer.User, [2]*http.Cookie) {
	t.Helper()
	users, ada := twoUserStore(t)
	handler := newGraphServer(t, server.Config{
		Contacts:          newFakeContactStore(),
		Users:             users,
		Live:              hub,
		Version:           "9.9.9",
		MaxStreamLifetime: lifetime,
		MaxStreamsPerUser: subscriptionSlots,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, ada, twoUserCookies(t, handler)
}

// openSubscription posts a subscription to the graph over the event stream transport.
func openSubscription(t *testing.T, srv *httptest.Server, cookie *http.Cookie) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), subscriptionPatience)
	t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, srv.URL+"/api/graphql", strings.NewReader(coreEventSubscription),
	)
	if err != nil {
		t.Fatalf("building the subscription request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := srv.Client().Do(request)
	if err != nil {
		t.Fatalf("opening the subscription: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

// subscriptionFrames returns a reader over an accepted subscription's frames.
func subscriptionFrames(t *testing.T, response *http.Response) *bufio.Reader {
	t.Helper()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("subscription status = %d, want %d", response.StatusCode, http.StatusOK)
	}
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

func TestCoreEventSubscriptionDeliversOnlyTheSubscribersFrames(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	srv, assignee, cookies := subscribingServer(t, hub, time.Minute)
	addressed := subscriptionFrames(t, openSubscription(t, srv, cookies[0]))
	unaddressed := subscriptionFrames(t, openSubscription(t, srv, cookies[1]))
	waitForSubscribers(t, hub, 2)

	hub.Broadcast(event.Frame{Name: event.TaskCreated, Audience: assignee.ID})
	hub.Broadcast(event.Frame{Name: event.ContactCreated})

	if got := nextData(t, addressed); !strings.Contains(got, "task.created") {
		t.Errorf("the assignee read %q, want it to carry task.created", got)
	}
	if got := nextData(t, unaddressed); !strings.Contains(got, "contact.created") {
		t.Errorf("the other subscriber read %q, want the targeted name withheld", got)
	}
}

func TestAnonymousSubscriptionIsRejected(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	srv, _, _ := subscribingServer(t, hub, time.Minute)

	frames := subscriptionFrames(t, openSubscription(t, srv, nil))

	if got := nextData(t, frames); !strings.Contains(got, "UNAUTHENTICATED") {
		t.Errorf("anonymous subscription read %q, want the gate's rejection", got)
	}
	if hub.Subscribers() != 0 {
		t.Error("an anonymous subscription reached the hub, want the gate to stop it")
	}
}

func TestSubscriptionRejectsTheSixthConcurrentStream(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	srv, _, cookies := subscribingServer(t, hub, time.Minute)
	for range subscriptionSlots {
		subscriptionFrames(t, openSubscription(t, srv, cookies[0]))
	}
	waitForSubscribers(t, hub, subscriptionSlots)

	rejected := openSubscription(t, srv, cookies[0])

	if rejected.StatusCode != http.StatusTooManyRequests {
		t.Errorf("sixth subscription = %d, want %d", rejected.StatusCode, http.StatusTooManyRequests)
	}
	accepted := openSubscription(t, srv, cookies[1])
	if accepted.StatusCode != http.StatusOK {
		t.Errorf("another user's subscription = %d, want %d", accepted.StatusCode, http.StatusOK)
	}
}

func TestSubscriptionClosesAtTheStreamLifetime(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	srv, _, cookies := subscribingServer(t, hub, 700*time.Millisecond)
	frames := subscriptionFrames(t, openSubscription(t, srv, cookies[0]))

	drained := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(frames)
		drained <- err
	}()

	select {
	case err := <-drained:
		if err != nil {
			t.Fatalf("draining the closed subscription: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the subscription outlived its stream lifetime, want the deadline to close it")
	}
	if hub.Subscribers() != 0 {
		t.Error("the subscription outlived its stream on the hub, want it removed")
	}
}

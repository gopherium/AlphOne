// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/graphres"
)

// subscriptionPatience bounds how long a subscription test waits on a frame.
const subscriptionPatience = 5 * time.Second

// subscriberContext returns a cancellable context acting as the given user.
func subscriberContext(t *testing.T, user uuid.UUID) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(authkit.WithIdentity(t.Context(), authkit.Identity{ID: user}))
	t.Cleanup(cancel)
	return ctx, cancel
}

// nextName returns the next name a subscription streams, failing when none arrives.
func nextName(t *testing.T, names <-chan string) string {
	t.Helper()
	select {
	case name, ok := <-names:
		if !ok {
			t.Fatal("the subscription closed, want a name")
		}
		return name
	case <-time.After(subscriptionPatience):
		t.Fatal("no name arrived, want the broadcast one")
		return ""
	}
}

func TestCoreEventRefusesAGraphWithoutABroadcaster(t *testing.T) {
	t.Parallel()

	resolver := &graphres.Resolver{}
	ctx, _ := subscriberContext(t, uuid.Must(uuid.NewV7()))

	names, err := resolver.SubscriptionResolvers().CoreEvent(ctx)

	if names != nil {
		t.Error("a graph without a broadcaster handed out a channel, want none")
	}
	if err == nil {
		t.Fatal("CoreEvent() error = nil, want the unavailable complaint")
	}
}

func TestCoreEventStreamsTheFramesTheSubscriberMaySee(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	resolver := &graphres.Resolver{Live: hub}
	user := uuid.Must(uuid.NewV7())
	ctx, _ := subscriberContext(t, user)

	names, err := resolver.SubscriptionResolvers().CoreEvent(ctx)
	if err != nil {
		t.Fatalf("CoreEvent() error = %v, want nil", err)
	}
	waitForSubscriber(t, hub)
	hub.Broadcast(event.Frame{Name: event.TaskCreated, Audience: user})

	if got := nextName(t, names); got != string(event.TaskCreated) {
		t.Errorf("streamed name = %q, want %q", got, event.TaskCreated)
	}
}

func TestCoreEventClosesWhenTheSubscriberGoesAway(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	resolver := &graphres.Resolver{Live: hub}
	ctx, cancel := subscriberContext(t, uuid.Must(uuid.NewV7()))

	names, err := resolver.SubscriptionResolvers().CoreEvent(ctx)
	if err != nil {
		t.Fatalf("CoreEvent() error = %v, want nil", err)
	}
	waitForSubscriber(t, hub)
	cancel()

	select {
	case _, ok := <-names:
		if ok {
			t.Error("a name arrived after the subscriber went away, want the channel closed")
		}
	case <-time.After(subscriptionPatience):
		t.Fatal("the channel stayed open, want it closed once the subscriber went away")
	}
	waitForNoSubscribers(t, hub)
}

func TestCoreEventStopsForwardingOnceTheSubscriberGoesAway(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	resolver := &graphres.Resolver{Live: hub}
	user := uuid.Must(uuid.NewV7())
	ctx, cancel := subscriberContext(t, user)

	names, err := resolver.SubscriptionResolvers().CoreEvent(ctx)
	if err != nil {
		t.Fatalf("CoreEvent() error = %v, want nil", err)
	}
	waitForSubscriber(t, hub)
	for range subscriptionOverflow {
		hub.Broadcast(event.Frame{Name: event.TaskCreated, Audience: user})
	}
	cancel()

	drain(names)
	waitForNoSubscribers(t, hub)
}

// subscriptionOverflow is more frames than one subscription buffers.
const subscriptionOverflow = 32

// drain reads a channel until it closes.
func drain(names <-chan string) {
	for range names {
	}
}

// waitForSubscriber blocks until the hub holds one subscriber.
func waitForSubscriber(t *testing.T, hub *event.Hub) {
	t.Helper()
	waitForSubscribers(t, hub, 1)
}

// waitForNoSubscribers blocks until the hub holds none.
func waitForNoSubscribers(t *testing.T, hub *event.Hub) {
	t.Helper()
	waitForSubscribers(t, hub, 0)
}

// waitForSubscribers blocks until the hub holds the wanted subscriber count.
func waitForSubscribers(t *testing.T, hub *event.Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(subscriptionPatience)
	for time.Now().Before(deadline) {
		if hub.Subscribers() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("subscribers = %d, want %d", hub.Subscribers(), want)
}

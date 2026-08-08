// SPDX-License-Identifier: Elastic-2.0

package event_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
)

// maria and noah stand in for two signed in users.
var (
	maria = uuid.MustParse("0198c000-0000-7000-8000-000000000001")
	noah  = uuid.MustParse("0198c000-0000-7000-8000-000000000002")
)

func TestHubDeliversSharedFramesToAllSubscribers(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	first := hub.Subscribe(maria)
	second := hub.Subscribe(noah)

	hub.Broadcast(event.Frame{Name: event.ContactCreated})

	if got := <-first; got != event.ContactCreated {
		t.Errorf("first subscriber got %q, want %q", got, event.ContactCreated)
	}
	if got := <-second; got != event.ContactCreated {
		t.Errorf("second subscriber got %q, want %q", got, event.ContactCreated)
	}
}

func TestHubDeliversATargetedFrameOnlyToItsAudience(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	hers := hub.Subscribe(maria)
	his := hub.Subscribe(noah)

	hub.Broadcast(event.Frame{Name: event.TaskCreated, Audience: maria})

	if got := <-hers; got != event.TaskCreated {
		t.Errorf("the assignee got %q, want %q", got, event.TaskCreated)
	}
	if len(his) != 0 {
		t.Errorf("the other subscriber buffered %d frames, want none", len(his))
	}
}

func TestHubCountsItsSubscribers(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	subscription := hub.Subscribe(maria)

	if got := hub.Subscribers(); got != 1 {
		t.Errorf("Subscribers() = %d, want 1", got)
	}
	hub.Unsubscribe(subscription)
	if got := hub.Subscribers(); got != 0 {
		t.Errorf("Subscribers() after unsubscribe = %d, want 0", got)
	}
}

func TestHubStopsDeliveringAfterUnsubscribe(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	subscription := hub.Subscribe(maria)
	hub.Unsubscribe(subscription)

	hub.Broadcast(event.Frame{Name: event.ContactCreated})

	if _, open := <-subscription; open {
		t.Error("subscription still open, want it closed on unsubscribe")
	}
}

func TestHubBroadcastDoesNotBlockOnAFullSubscriber(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	stalled := hub.Subscribe(maria)
	listening := hub.Subscribe(noah)

	for range 32 {
		hub.Broadcast(event.Frame{Name: event.ContactCreated})
	}

	if got := len(stalled); got == 32 {
		t.Errorf("stalled subscriber buffered %d events, want the overflow dropped", got)
	}
	if got := <-listening; got != event.ContactCreated {
		t.Errorf("listening subscriber got %q, want %q", got, event.ContactCreated)
	}
}

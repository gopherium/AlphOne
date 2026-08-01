// SPDX-License-Identifier: Elastic-2.0

package event_test

import (
	"testing"

	"github.com/gopherium/alphone/internal/event"
)

func TestHubDeliversNamesToAllSubscribers(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	first := hub.Subscribe()
	second := hub.Subscribe()

	hub.Broadcast(event.TaskCreated)

	if got := <-first; got != event.TaskCreated {
		t.Errorf("first subscriber got %q, want %q", got, event.TaskCreated)
	}
	if got := <-second; got != event.TaskCreated {
		t.Errorf("second subscriber got %q, want %q", got, event.TaskCreated)
	}
}

func TestHubCountsItsSubscribers(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	subscription := hub.Subscribe()

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
	subscription := hub.Subscribe()
	hub.Unsubscribe(subscription)

	hub.Broadcast(event.ContactCreated)

	if _, open := <-subscription; open {
		t.Error("subscription still open, want it closed on unsubscribe")
	}
}

func TestHubBroadcastDoesNotBlockOnAFullSubscriber(t *testing.T) {
	t.Parallel()

	hub := event.NewHub()
	stalled := hub.Subscribe()
	listening := hub.Subscribe()

	for range 32 {
		hub.Broadcast(event.TaskCreated)
	}

	if got := len(stalled); got == 32 {
		t.Errorf("stalled subscriber buffered %d events, want the overflow dropped", got)
	}
	if got := <-listening; got != event.TaskCreated {
		t.Errorf("listening subscriber got %q, want %q", got, event.TaskCreated)
	}
}

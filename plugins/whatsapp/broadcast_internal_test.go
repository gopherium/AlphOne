// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBroadcasterDeliversEventsToAllSubscribers(t *testing.T) {
	t.Parallel()

	b := newBroadcaster()
	tenant := uuid.Must(uuid.NewV7())
	first := b.subscribe(tenant)
	second := b.subscribe(tenant)
	want := event{Conversation: uuid.Must(uuid.NewV7()), Tenant: tenant}

	b.broadcast(want)

	for name, ch := range map[string]chan event{"first": first, "second": second} {
		select {
		case got := <-ch:
			if got != want {
				t.Errorf("%s subscriber got %+v, want %+v", name, got, want)
			}
		default:
			t.Errorf("%s subscriber received nothing", name)
		}
	}
}

func TestBroadcasterStopsDeliveringAfterUnsubscribe(t *testing.T) {
	t.Parallel()

	b := newBroadcaster()
	tenant := uuid.Must(uuid.NewV7())
	ch := b.subscribe(tenant)
	b.unsubscribe(ch)

	b.broadcast(event{Conversation: uuid.Must(uuid.NewV7()), Tenant: tenant})

	if _, open := <-ch; open {
		t.Fatal("received an event after unsubscribe, want a drained and closed channel")
	}
}

func TestBroadcastDoesNotBlockOnAFullSubscriber(t *testing.T) {
	t.Parallel()

	b := newBroadcaster()
	tenant := uuid.Must(uuid.NewV7())
	b.subscribe(tenant) // deliberately never drained

	done := make(chan struct{})
	go func() {
		for range 100 {
			b.broadcast(event{Conversation: uuid.Must(uuid.NewV7()), Tenant: tenant})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on a full subscriber, want a non-blocking drop")
	}
}

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
	first := b.subscribe()
	second := b.subscribe()
	want := event{Conversation: uuid.Must(uuid.NewV7())}

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
	ch := b.subscribe()
	b.unsubscribe(ch)

	b.broadcast(event{Conversation: uuid.Must(uuid.NewV7())})

	if _, open := <-ch; open {
		t.Fatal("received an event after unsubscribe, want a drained and closed channel")
	}
}

func TestBroadcastDoesNotBlockOnAFullSubscriber(t *testing.T) {
	t.Parallel()

	b := newBroadcaster()
	b.subscribe() // deliberately never drained

	done := make(chan struct{})
	go func() {
		for range 100 {
			b.broadcast(event{Conversation: uuid.Must(uuid.NewV7())})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on a full subscriber, want a non-blocking drop")
	}
}

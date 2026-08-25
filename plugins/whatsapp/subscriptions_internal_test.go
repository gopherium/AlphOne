// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"testing"
	"time"

	"github.com/gopherium/alphone/sdk"
)

// pumpPatience bounds how long a pump test waits on the goroutine to return.
const pumpPatience = 5 * time.Second

// waitForSubscription blocks until the broadcaster holds one subscriber.
func waitForSubscription(t *testing.T, events *broadcaster) {
	t.Helper()
	deadline := time.Now().Add(pumpPatience)
	for time.Now().Before(deadline) {
		events.mu.Lock()
		held := len(events.subs)
		events.mu.Unlock()
		if held == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the broadcaster holds no subscriber, want the pump subscribed")
}

func TestSendOrDoneStopsOnceTheSubscriberGoesAway(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	unread := make(chan int)

	if sendOrDone(ctx, unread, 1) {
		t.Error("sendOrDone reported a send, want it to stop once the subscriber went away")
	}
}

func TestPumpEventsStopsWhenEmitStops(t *testing.T) {
	t.Parallel()

	events := newBroadcaster()
	out := make(chan int, 1)
	handled := make(chan struct{}, 1)
	returned := make(chan struct{})
	go func() {
		pumpEvents(t.Context(), events, out, func(event) bool {
			handled <- struct{}{}
			return false
		})
		close(returned)
	}()
	waitForSubscription(t, events)

	events.broadcast(event{Tenant: sdk.DefaultTenantID})

	select {
	case <-handled:
	case <-time.After(pumpPatience):
		t.Fatal("the pump handed nothing to emit, want the broadcast event")
	}
	select {
	case <-returned:
	case <-time.After(pumpPatience):
		t.Fatal("the pump kept running after emit stopped, want it to return")
	}
	if _, open := <-out; open {
		t.Error("the pump left its channel open, want it closed once emit stopped")
	}
}

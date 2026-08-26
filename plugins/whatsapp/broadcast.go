// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"sync"

	"github.com/google/uuid"
)

// subscriberBuffer is how many events a subscriber can lag behind.
const subscriberBuffer = 8

// event notifies subscribers that a conversation changed in one tenant.
type event struct {
	// Conversation is the conversation the change belongs to.
	Conversation uuid.UUID
	// Message is the arrival the change carries. Nil for every other change.
	Message *messageRow
	// Tenant is the tenant the change belongs to.
	Tenant uuid.UUID
}

// broadcaster fans out events to every current subscriber of their tenant.
// A subscriber that falls behind misses events rather than stalling the sender.
type broadcaster struct {
	mu   sync.Mutex
	subs map[chan event]uuid.UUID
}

// newBroadcaster creates a broadcaster with an empty subscriber set.
func newBroadcaster() *broadcaster {
	return &broadcaster{subs: make(map[chan event]uuid.UUID)}
}

// subscribe registers a new subscriber in its tenant and returns its buffered channel.
func (b *broadcaster) subscribe(tenant uuid.UUID) chan event {
	ch := make(chan event, subscriberBuffer)
	b.mu.Lock()
	b.subs[ch] = tenant
	b.mu.Unlock()
	return ch
}

// unsubscribe removes ch and closes it, ending any range over it.
func (b *broadcaster) unsubscribe(ch chan event) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
	close(ch)
}

// broadcast delivers e to every subscriber of its tenant with room, dropping
// it for any whose buffer is full.
func (b *broadcaster) broadcast(e event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch, tenant := range b.subs {
		if e.Tenant != tenant {
			continue
		}
		select {
		case ch <- e:
		default:
		}
	}
}

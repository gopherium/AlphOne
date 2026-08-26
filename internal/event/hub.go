// SPDX-License-Identifier: Elastic-2.0

package event

import (
	"sync"

	"github.com/google/uuid"
)

// subscriberBuffer is how many names a subscriber can lag behind.
const subscriberBuffer = 8

// Frame is one announced event beside the user and tenant it concerns.
type Frame struct {
	// Name is the announced event.
	Name Name
	// Audience is the only user the frame reaches. Nil reaches everyone.
	Audience uuid.UUID
	// Tenant is the only tenant the frame reaches. Nil reaches every tenant.
	Tenant uuid.UUID
}

// listener is one subscription beside the user and tenant listening on it.
type listener struct {
	names  chan Name
	user   uuid.UUID
	tenant uuid.UUID
}

// Hub fans out published frames to every subscriber they concern.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Name]listener
}

// NewHub returns a [Hub] with an empty subscriber set.
func NewHub() *Hub {
	return &Hub{subs: make(map[chan Name]listener)}
}

// Subscribe registers user as a subscriber in its tenant and returns its buffered channel.
func (h *Hub) Subscribe(user, tenant uuid.UUID) chan Name {
	ch := make(chan Name, subscriberBuffer)
	h.mu.Lock()
	h.subs[ch] = listener{names: ch, user: user, tenant: tenant}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes ch and closes it.
func (h *Hub) Unsubscribe(ch chan Name) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

// Subscribers returns how many subscriptions are currently registered.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Broadcast delivers frame to every subscriber it concerns with room, dropping
// it for any whose buffer is full.
func (h *Hub) Broadcast(frame Frame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sub := range h.subs {
		if frame.Audience != uuid.Nil && frame.Audience != sub.user {
			continue
		}
		if frame.Tenant != uuid.Nil && frame.Tenant != sub.tenant {
			continue
		}
		select {
		case sub.names <- frame.Name:
		default:
		}
	}
}

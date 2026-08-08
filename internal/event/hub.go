// SPDX-License-Identifier: Elastic-2.0

package event

import (
	"sync"

	"github.com/google/uuid"
)

// subscriberBuffer is how many names a subscriber can lag behind.
const subscriberBuffer = 8

// Frame is one announced event beside the user it concerns.
type Frame struct {
	// Name is the announced event.
	Name Name
	// Audience is the only user the frame reaches. Nil reaches everyone.
	Audience uuid.UUID
}

// listener is one subscription beside the user listening on it.
type listener struct {
	names chan Name
	user  uuid.UUID
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

// Subscribe registers user as a subscriber and returns its buffered channel.
func (h *Hub) Subscribe(user uuid.UUID) chan Name {
	ch := make(chan Name, subscriberBuffer)
	h.mu.Lock()
	h.subs[ch] = listener{names: ch, user: user}
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
		select {
		case sub.names <- frame.Name:
		default:
		}
	}
}

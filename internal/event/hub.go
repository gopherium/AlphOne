// SPDX-License-Identifier: Elastic-2.0

package event

import "sync"

// subscriberBuffer is how many names a subscriber can lag behind.
const subscriberBuffer = 8

// Hub fans out published event names to every current subscriber.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Name]struct{}
}

// NewHub returns a [Hub] with an empty subscriber set.
func NewHub() *Hub {
	return &Hub{subs: make(map[chan Name]struct{})}
}

// Subscribe registers a new subscriber and returns its buffered channel.
func (h *Hub) Subscribe() chan Name {
	ch := make(chan Name, subscriberBuffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
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

// Broadcast delivers name to every subscriber with room, dropping it for any
// whose buffer is full.
func (h *Hub) Broadcast(name Name) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- name:
		default:
		}
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later

package whatsapp

import (
	"sync"

	"github.com/google/uuid"
)

// event notifies subscribers that a conversation changed.
type event struct {
	Conversation uuid.UUID `json:"conversation"`
}

// broadcaster fans out events to every current subscriber. A subscriber
// that falls behind misses events rather than stalling the sender.
type broadcaster struct {
	mu   sync.Mutex
	subs map[chan event]struct{}
}

// newBroadcaster creates a broadcaster with an empty subscriber set.
func newBroadcaster() *broadcaster {
	return &broadcaster{subs: make(map[chan event]struct{})}
}

// subscribe registers a new subscriber and returns its buffered channel.
func (b *broadcaster) subscribe() chan event {
	ch := make(chan event, 8)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
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

// broadcast delivers e to every subscriber with room, dropping it for any
// whose buffer is full.
func (b *broadcaster) broadcast(e event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

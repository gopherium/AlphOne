// SPDX-License-Identifier: Elastic-2.0

package server

import "sync"

// addressLimiter caps the concurrent requests of callers with no identity per client address and across every address.
type addressLimiter struct {
	mu         sync.Mutex
	perAddress int
	ceiling    int
	total      int
	counts     map[string]int
}

// newAddressLimiter returns an addressLimiter admitting perAddress requests from one address and ceiling in all.
func newAddressLimiter(perAddress, ceiling int) *addressLimiter {
	return &addressLimiter{perAddress: perAddress, ceiling: ceiling, counts: map[string]int{}}
}

// acquire reserves a request slot for the address, reporting whether one was free.
func (l *addressLimiter) acquire(address string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= l.ceiling || l.counts[address] >= l.perAddress {
		return false
	}
	l.counts[address]++
	l.total++
	return true
}

// release frees a request slot the address held.
func (l *addressLimiter) release(address string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[address]--
	l.total--
	if l.counts[address] == 0 {
		delete(l.counts, address)
	}
}

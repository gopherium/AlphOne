// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gopherium/gouncer/authkit"
)

// DefaultMaxStreamLifetime and DefaultMaxStreamsPerUser bound the streams of a config that leaves them zero.
const (
	DefaultMaxStreamLifetime = 5 * time.Minute
	DefaultMaxStreamsPerUser = 5
)

// streamDefaults resolves the configured stream bounds, filling zero values
// with the safe defaults.
func streamDefaults(cfg Config) (time.Duration, int) {
	lifetime := cfg.MaxStreamLifetime
	if lifetime == 0 {
		lifetime = DefaultMaxStreamLifetime
	}
	limit := cfg.MaxStreamsPerUser
	if limit == 0 {
		limit = DefaultMaxStreamsPerUser
	}
	return lifetime, limit
}

// streamLimiter caps concurrent protected plugin requests per user.
type streamLimiter struct {
	mu     sync.Mutex
	limit  int
	counts map[uuid.UUID]int
}

// newStreamLimiter returns a streamLimiter admitting limit concurrent
// requests per user.
func newStreamLimiter(limit int) *streamLimiter {
	return &streamLimiter{limit: limit, counts: map[uuid.UUID]int{}}
}

// acquire reserves a request slot for the user, reporting whether one was
// free.
func (l *streamLimiter) acquire(userID uuid.UUID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[userID] >= l.limit {
		return false
	}
	l.counts[userID]++
	return true
}

// release frees a request slot held by the user.
func (l *streamLimiter) release(userID uuid.UUID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[userID]--
	if l.counts[userID] == 0 {
		delete(l.counts, userID)
	}
}

// boundPluginRequest limits a protected plugin request to the host stream
// lifetime and the per-user concurrency budget.
func (s *server) boundPluginRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := authkit.IdentityFromContext(r.Context())
		if !s.streams.acquire(user.ID) {
			w.Header().Set("Retry-After", strconv.Itoa(int(s.maxStreamLifetime.Seconds())))
			authkit.RespondError(w, http.StatusTooManyRequests,
				authkit.ErrorResponse{Message: "too many concurrent requests"})
			return
		}
		defer s.streams.release(user.ID)
		ctx, cancel := context.WithTimeout(r.Context(), s.maxStreamLifetime)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

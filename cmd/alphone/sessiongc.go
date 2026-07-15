// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"log/slog"
	"time"
)

// sessionGCInterval is how often expired sessions are swept, and
// sessionGCTimeout bounds each sweep.
const (
	sessionGCInterval = time.Hour
	sessionGCTimeout  = 30 * time.Second
)

// expiredSessionReaper deletes sessions that have expired.
type expiredSessionReaper interface {
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
}

// reapExpiredSessions sweeps expired sessions once, then every interval
// until ctx is cancelled, bounding each sweep to timeout.
func reapExpiredSessions(
	ctx context.Context,
	reaper expiredSessionReaper,
	interval time.Duration,
	timeout time.Duration,
	log *slog.Logger,
) {
	reapOnce(ctx, reaper, timeout, log)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reapOnce(ctx, reaper, timeout, log)
		}
	}
}

// reapOnce deletes the currently expired sessions within timeout, logging the outcome.
func reapOnce(ctx context.Context, reaper expiredSessionReaper, timeout time.Duration, log *slog.Logger) {
	sweepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	count, err := reaper.DeleteExpiredSessions(sweepCtx, time.Now().UTC())
	if err != nil {
		if ctx.Err() == nil {
			log.Error("reap expired sessions", "error", err)
		}
		return
	}
	if count > 0 {
		log.Info("reaped expired sessions", "count", count)
	}
}

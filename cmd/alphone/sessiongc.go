// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"log/slog"
	"time"
)

// sessionGCInterval is how often expired sessions are swept.
const sessionGCInterval = time.Hour

// expiredSessionReaper deletes sessions that have expired.
type expiredSessionReaper interface {
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
}

// reapExpiredSessions sweeps expired sessions once, then every interval
// until ctx is cancelled.
func reapExpiredSessions(
	ctx context.Context,
	reaper expiredSessionReaper,
	interval time.Duration,
	log *slog.Logger,
) {
	reapOnce(ctx, reaper, log)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reapOnce(ctx, reaper, log)
		}
	}
}

// reapOnce deletes the currently expired sessions, logging the outcome.
func reapOnce(ctx context.Context, reaper expiredSessionReaper, log *slog.Logger) {
	count, err := reaper.DeleteExpiredSessions(ctx, time.Now().UTC())
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

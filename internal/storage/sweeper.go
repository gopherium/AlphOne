// SPDX-License-Identifier: Elastic-2.0

package storage

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/postgresblob"
)

// Sweeper deletes the unfinished uploads of the postgres driver on a timer while it runs.
type Sweeper struct {
	// pool reaches the tables of the postgres driver.
	pool *pgxpool.Pool
	// options name the uploads one sweep deletes.
	options postgresblob.SweepOptions
	// interval is how long the sweeper waits between two sweeps.
	interval time.Duration
	// enabled reports whether the files live in PostgreSQL.
	enabled bool
	// logger records what each sweep deleted and what failed.
	logger *slog.Logger
	// cancel ends the sweeps, nil until Start.
	cancel context.CancelFunc
	// done closes once the sweeps ended.
	done chan struct{}
}

// NewSweeper returns the sweeper the settings name over pool, one that sweeps only under the postgres driver.
func NewSweeper(settings Settings, pool *pgxpool.Pool, logger *slog.Logger) *Sweeper {
	return &Sweeper{
		pool:     pool,
		options:  postgresblob.SweepOptions{Schema: Schema, OlderThan: settings.SweepAge, Limit: settings.SweepBatch},
		interval: settings.SweepInterval,
		enabled:  settings.Driver == Postgres,
		logger:   logger,
	}
}

// Start sweeps once and then once every interval in the background, nothing under a driver other than postgres.
func (s *Sweeper) Start() {
	if !s.enabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		s.run(ctx)
	}()
}

// Stop ends the sweeps and waits for the one running, nothing when none started.
func (s *Sweeper) Stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	<-s.done
}

// run sweeps at once and then on every tick until ctx ends.
func (s *Sweeper) run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		s.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sweep deletes one batch of unfinished uploads and logs how many went, or why none could.
func (s *Sweeper) sweep(ctx context.Context) {
	deleted, err := postgresblob.Sweep(ctx, s.pool, s.options)
	switch {
	case err != nil && ctx.Err() == nil:
		s.logger.Error("sweeping unfinished uploads", "error", err)
	case deleted > 0:
		s.logger.Info("swept unfinished uploads", "deleted", deleted)
	}
}

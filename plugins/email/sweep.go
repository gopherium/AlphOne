// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// queuedFile is one file the queue holds for deletion, beside the workspace that keeps it.
type queuedFile struct {
	// tenant names the workspace that keeps the file.
	tenant uuid.UUID
	// id names the file.
	id uuid.UUID
}

// sweeper deletes on a timer the files no stored mail holds, in every workspace in turn.
type sweeper struct {
	// pool reaches the queue of every workspace.
	pool *pgxpool.Pool
	// store deletes the bytes of each file.
	store sdk.FileStore
	// logger records what a sweep deleted and what failed.
	logger *slog.Logger
	// pace sets the interval, the batch, the budget and the orphan age of the sweeps.
	pace sweepSettings
	// cancel ends the sweeps, nil until Start.
	cancel context.CancelFunc
	// done closes once the sweeps ended.
	done chan struct{}
}

// Start sweeps once every interval in the background, the first time one interval after it starts.
func (s *sweeper) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		s.run(ctx)
	}()
}

// Stop ends the sweeps and waits for the one running, answering the error of ctx when ctx ends first.
func (s *sweeper) Stop(ctx context.Context) error {
	if s.cancel == nil {
		return nil
	}
	s.cancel()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run sweeps on every tick of the interval until ctx ends.
func (s *sweeper) run(ctx context.Context) {
	ticker := time.NewTicker(s.pace.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep marks the files stopped runs left behind for deletion, then deletes the queue, within the budget.
func (s *sweeper) sweep(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, s.pace.budget)
	defer cancel()
	s.queueLeftBehind(ctx)
	if deleted := s.deleteQueue(ctx); deleted > 0 {
		s.logger.InfoContext(ctx, "email: swept the files no stored mail holds", "deleted", deleted)
	}
}

// queueLeftBehind marks the files stopped runs left behind for deletion, statement after statement, until none is left.
func (s *sweeper) queueLeftBehind(ctx context.Context) {
	for ctx.Err() == nil {
		marked, err := s.markLeftBehind(ctx)
		if err != nil {
			s.logFailure(ctx, "email: could not mark the files left behind for deletion", "error", err)
			return
		}
		if marked == 0 {
			return
		}
	}
}

// markLeftBehind marks one batch of each workspace's writing files past the orphan age for deletion and counts them.
func (s *sweeper) markLeftBehind(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE plugin_email.pending_files queued SET state = 'deleting'
		FROM (
			SELECT taken.tenant_id, taken.file_id
			FROM (SELECT DISTINCT tenant_id FROM plugin_email.pending_files
				WHERE state = 'writing' AND created_at < now() - $1::interval) aged
			CROSS JOIN LATERAL (
				SELECT left_behind.tenant_id, left_behind.file_id FROM plugin_email.pending_files left_behind
				WHERE left_behind.tenant_id = aged.tenant_id AND left_behind.state = 'writing'
					AND left_behind.created_at < now() - $1::interval
				ORDER BY left_behind.created_at, left_behind.file_id
				LIMIT $2
				FOR UPDATE SKIP LOCKED
			) taken
		) picked
		WHERE queued.tenant_id = picked.tenant_id AND queued.file_id = picked.file_id`,
		s.pace.orphanAge, s.pace.batch)
	if err != nil {
		return 0, fmt.Errorf("email: mark the files left behind: %w", err)
	}
	return tag.RowsAffected(), nil
}

// deleteQueue deletes each queued file no stored mail holds, then each row, round by round, answering how many went.
func (s *sweeper) deleteQueue(ctx context.Context) int {
	var failed []uuid.UUID
	deleted := 0
	for ctx.Err() == nil {
		queued, err := s.readQueue(ctx, failed)
		if err != nil {
			s.logFailure(ctx, "email: could not read the file queue", "error", err)
			return deleted
		}
		if len(queued) == 0 {
			return deleted
		}
		gone, refused := s.deleteRound(ctx, queued)
		deleted += gone
		failed = append(failed, refused...)
	}
	return deleted
}

// deleteRound deletes the files of one round until the sweep's time runs out, answering how many went and which failed.
func (s *sweeper) deleteRound(ctx context.Context, queued []queuedFile) (int, []uuid.UUID) {
	deleted := 0
	var failed []uuid.UUID
	for _, file := range queued {
		if ctx.Err() != nil {
			break
		}
		gone, err := s.deleteFile(ctx, file)
		if err != nil {
			s.logFailure(ctx, "email: could not delete a queued file",
				"workspace", file.tenant, "file", file.id, "error", err)
			failed = append(failed, file.id)
			continue
		}
		if gone {
			deleted++
		}
	}
	return deleted, failed
}

// readQueue returns one batch of each workspace's queued files, round by round, leaving out the files in failed.
func (s *sweeper) readQueue(ctx context.Context, failed []uuid.UUID) ([]queuedFile, error) {
	rows, _ := s.pool.Query(ctx, `SELECT tenant_id, file_id FROM (
			SELECT tenant_id, file_id,
				row_number() OVER (PARTITION BY tenant_id ORDER BY created_at, file_id) AS turn
			FROM plugin_email.pending_files
			WHERE state = 'deleting' AND file_id <> ALL(coalesce($2::uuid[], '{}'))
		) queued
		WHERE turn <= $1
		ORDER BY turn, tenant_id`,
		s.pace.batch, failed)
	queued, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (queuedFile, error) {
		var file queuedFile
		err := row.Scan(&file.tenant, &file.id)
		return file, err
	})
	if err != nil {
		return nil, fmt.Errorf("email: read the file queue: %w", err)
	}
	return queued, nil
}

// deleteFile deletes a queued file no stored mail of its workspace holds, then its row, answering whether it went.
func (s *sweeper) deleteFile(ctx context.Context, file queuedFile) (bool, error) {
	holders, err := s.countFileHolders(ctx, file)
	if err != nil {
		return false, err
	}
	if holders == 0 {
		err = s.store.Delete(sdk.WithTenant(ctx, file.tenant), file.id)
	}
	if err != nil && !errors.Is(err, sdk.ErrFileNotFound) {
		return false, fmt.Errorf("email: delete a queued file: %w", err)
	}
	_, err = s.pool.Exec(ctx, "DELETE FROM plugin_email.pending_files WHERE tenant_id = $1 AND file_id = $2",
		file.tenant, file.id)
	if err != nil {
		return false, fmt.Errorf("email: remove a file from the queue: %w", err)
	}
	return holders == 0, nil
}

// countFileHolders counts the stored mails and attachments of the workspace of a queued file that hold it.
func (s *sweeper) countFileHolders(ctx context.Context, file queuedFile) (int64, error) {
	var holders int64
	err := s.pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM plugin_email.messages m WHERE m.tenant_id = $1 AND m.raw_file_id = $2)
			+ (SELECT count(*) FROM plugin_email.attachments a WHERE a.tenant_id = $1 AND a.file_id = $2)`,
		file.tenant, file.id,
	).Scan(&holders)
	if err != nil {
		return 0, fmt.Errorf("email: count the holders of a queued file: %w", err)
	}
	return holders, nil
}

// logFailure logs a failure of a sweep at error level unless the sweep's own time ran out or it was stopped.
func (s *sweeper) logFailure(ctx context.Context, msg string, args ...any) {
	if ctx.Err() == nil {
		s.logger.ErrorContext(ctx, msg, args...)
	}
}

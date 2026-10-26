// SPDX-License-Identifier: Elastic-2.0

package storage_test

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob"

	"github.com/gopherium/alphone/internal/storage"
)

// sweepBudget bounds how long a test waits for the first sweep.
const sweepBudget = 10 * time.Second

// pendingRowsOf counts the pending rows of the key its one argument names.
const pendingRowsOf = "SELECT count(*) FROM storage.objects WHERE key = $1 AND state = 'pending'"

// agePending makes the pending row of the key its first argument names look untouched for its second.
const agePending = "UPDATE storage.objects SET modified_at = now() - $2::interval WHERE key = $1 AND state = 'pending'"

// pendingRows counts every pending row of the storage schema.
const pendingRows = "SELECT count(*) FROM storage.objects WHERE state = 'pending'"

// lockedLog is a log the sweeper writes from its own goroutine while the test reads it.
type lockedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p under the lock.
func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// String returns what the log holds so far.
func (l *lockedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// pendingUpload starts an upload of key on bucket longer than one row and leaves it unfinished.
func pendingUpload(t *testing.T, bucket *blob.Bucket, key string) {
	t.Helper()
	writer, err := bucket.NewWriter(t.Context(), key, &blob.WriterOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("NewWriter() error = %v, want a writer", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Write([]byte(receiptText)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}
}

// waitFor fails the test unless done reports true within the sweep budget.
func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(sweepBudget)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("waited %v for %s", sweepBudget, what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// staleUploads leaves an unfinished upload under each key, untouched past the sweep age but within its interval.
func staleUploads(t *testing.T, settings storage.Settings, pool *pgxpool.Pool, keys ...string) *blob.Bucket {
	t.Helper()
	bucket, err := storage.Open(t.Context(), settings, pool)
	if err != nil {
		t.Fatalf("Open() error = %v, want the postgres bucket", err)
	}
	t.Cleanup(func() { _ = bucket.Close() })
	staleFor := (settings.SweepAge + settings.SweepInterval) / 2
	for _, key := range keys {
		pendingUpload(t, bucket, key)
		if _, err := pool.Exec(t.Context(), agePending, []byte(key), staleFor); err != nil {
			t.Fatalf("ageing the upload %s: %v", key, err)
		}
	}
	return bucket
}

// startedSweeper starts the sweeper the settings name over pool, logging to log, stopped when the test ends.
func startedSweeper(t *testing.T, settings storage.Settings, pool *pgxpool.Pool, log *lockedLog) *storage.Sweeper {
	t.Helper()
	sweeper := storage.NewSweeper(settings, pool, slog.New(slog.NewTextHandler(log, nil)))
	sweeper.Start()
	t.Cleanup(sweeper.Stop)
	return sweeper
}

func TestTheSweeperDeletesAnUnfinishedUploadPastItsAgeAndSparesAFreshOne(t *testing.T) {
	t.Parallel()

	pool := poolOn(t, migratedDatabase(t))
	settings := postgresSettings()
	bucket := staleUploads(t, settings, pool, "uploads/stale")
	pendingUpload(t, bucket, "uploads/fresh")
	var log lockedLog

	sweeper := startedSweeper(t, settings, pool, &log)
	waitFor(t, "the stale upload to go", func() bool {
		return count(t, pool, pendingRowsOf, []byte("uploads/stale")) == 0
	})
	sweeper.Stop()

	if left := count(t, pool, pendingRowsOf, []byte("uploads/fresh")); left != 1 {
		t.Errorf("the fresh upload holds %d pending rows after the sweep, want its one row spared", left)
	}
	if logged := log.String(); !strings.Contains(logged, "deleted=1") {
		t.Errorf("the log %q does not say one upload was deleted", logged)
	}
}

func TestOneSweepDeletesNoMoreUnfinishedUploadsThanItsBatch(t *testing.T) {
	t.Parallel()

	pool := poolOn(t, migratedDatabase(t))
	settings := postgresSettings()
	settings.SweepBatch = 2
	staleUploads(t, settings, pool, "uploads/first", "uploads/second", "uploads/third")
	var log lockedLog

	sweeper := startedSweeper(t, settings, pool, &log)
	waitFor(t, "the first sweep to be logged", func() bool { return strings.Contains(log.String(), "deleted=") })
	sweeper.Stop()

	if left := count(t, pool, pendingRows); left != 1 {
		t.Errorf("%d unfinished uploads are left after the first sweep, want the one past the batch of 2", left)
	}
	if logged := log.String(); !strings.Contains(logged, "deleted=2") {
		t.Errorf("the log %q does not say the batch of 2 uploads was deleted", logged)
	}
}

func TestTheSweeperSweepsAgainEveryInterval(t *testing.T) {
	t.Parallel()

	pool := poolOn(t, migratedDatabase(t))
	settings := postgresSettings()
	settings.SweepInterval, settings.SweepAge, settings.SweepBatch = 100*time.Millisecond, 50*time.Millisecond, 1
	staleUploads(t, settings, pool, "uploads/first", "uploads/second", "uploads/third")
	var log lockedLog

	startedSweeper(t, settings, pool, &log)

	waitFor(t, "three sweeps of one upload each", func() bool { return strings.Count(log.String(), "deleted=1") == 3 })
	if left := count(t, pool, pendingRows); left != 0 {
		t.Errorf("%d unfinished uploads are left after three sweeps of one, want none", left)
	}
}

func TestTheSweeperLogsASweepThatFails(t *testing.T) {
	t.Parallel()

	var log lockedLog

	startedSweeper(t, postgresSettings(), poolOn(t, bareDatabase(t)), &log)

	waitFor(t, "the failed sweep to be logged", func() bool { return strings.Contains(log.String(), "level=ERROR") })
	if logged := log.String(); !strings.Contains(logged, "storage") {
		t.Errorf("the log %q does not name the storage schema the sweep could not reach", logged)
	}
}

func TestTheSweeperSweepsNothingUnderTheS3Driver(t *testing.T) {
	t.Parallel()

	var log lockedLog
	sweeper := startedSweeper(t, s3Settings("https://files.example.com"), nil, &log)

	sweeper.Stop()
	sweeper.Stop()

	if logged := log.String(); logged != "" {
		t.Errorf("the sweeper of the s3 driver logged %q, want nothing swept", logged)
	}
}

func TestStoppingASweeperThatNeverStartedDoesNothing(t *testing.T) {
	t.Parallel()

	sweeper := storage.NewSweeper(postgresSettings(), nil, slog.New(slog.DiscardHandler))

	sweeper.Stop()
}

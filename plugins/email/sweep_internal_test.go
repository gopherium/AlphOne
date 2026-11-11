// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob/memblob"

	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/sdk"
)

// orphanAge is how long a file a stopped run left waits before the sweep of the sweep tests takes it.
const orphanAge = 24 * time.Hour

// sweepWait bounds how long a sweep test waits for the background loop.
const sweepWait = 10 * time.Second

// deletingStore hands every call to the host's store, running before first on each Delete and answering its error.
type deletingStore struct {
	sdk.FileStore
	// before runs ahead of each Delete, and its error replaces the delete.
	before func(ctx context.Context, id uuid.UUID) error
}

// Delete runs before and answers its error, deleting the file through the host's store when before answers nil.
func (d deletingStore) Delete(ctx context.Context, id uuid.UUID) error {
	if err := d.before(ctx, id); err != nil {
		return err
	}
	return d.FileStore.Delete(ctx, id)
}

// testSweeper returns a sweeper over a migrated database and the host's file store in memory, beside both and its log.
func testSweeper(t *testing.T, batch int) (*sweeper, *pgxpool.Pool, sdk.FileStore, *bytes.Buffer) {
	t.Helper()
	pool := newMigratedPool(t)
	store := storage.NewFileStore(memblob.OpenBucket(nil), "email", fileCap)
	logger, written := bufferLogger()
	return &sweeper{
		pool:   pool,
		store:  store,
		logger: logger,
		pace:   sweepSettings{interval: time.Hour, batch: batch, budget: time.Minute, orphanAge: orphanAge},
	}, pool, store, written
}

// saveFile saves content under id in the workspace tenant through store.
func saveFile(t *testing.T, store sdk.FileStore, tenant, id uuid.UUID, content string) {
	t.Helper()
	if _, err := store.SaveAs(sdk.WithTenant(t.Context(), tenant), id, "text/plain",
		strings.NewReader(content)); err != nil {
		t.Fatalf("saving the file %v: %v", id, err)
	}
}

// leaveFile saves a file in the workspace tenant and queues it in state, two days old, returning its id.
func leaveFile(t *testing.T, pool *pgxpool.Pool, store sdk.FileStore, tenant uuid.UUID, state string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	saveFile(t, store, tenant, id, "left behind")
	if _, err := pool.Exec(t.Context(), `INSERT INTO plugin_email.pending_files
		(tenant_id, file_id, state, size, created_at) VALUES ($1, $2, $3, 11, now() - interval '2 days')`,
		tenant, id, state); err != nil {
		t.Fatalf("queueing the file %v: %v", id, err)
	}
	return id
}

// fileGone reports whether store keeps no file under id in the workspace tenant.
func fileGone(t *testing.T, store sdk.FileStore, tenant, id uuid.UUID) bool {
	t.Helper()
	file, err := store.Open(sdk.WithTenant(t.Context(), tenant), id)
	if err == nil {
		_ = file.Close()
		return false
	}
	if !errors.Is(err, sdk.ErrFileNotFound) {
		t.Fatalf("opening the file %v: %v", id, err)
	}
	return true
}

// queueSize counts the queue rows of the workspace tenant.
func queueSize(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID) int {
	t.Helper()
	return countRows(t, pool, "SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1", tenant)
}

// errorLines returns the error lines a bufferLogger wrote.
func errorLines(t *testing.T, written *bytes.Buffer) []map[string]any {
	t.Helper()
	var held []map[string]any
	for _, line := range loggedLines(t, written) {
		if line[slog.LevelKey] == slog.LevelError.String() {
			held = append(held, line)
		}
	}
	return held
}

// holdFiles stores a mail of the workspace tenant that keeps the file raw as its raw copy and attachment as its file.
func holdFiles(t *testing.T, pool *pgxpool.Pool, tenant, raw, attachment uuid.UUID) {
	t.Helper()
	box := storeMailbox(t, pool, tenant)
	tx := beginTx(t, pool)
	row := mailRow(box, storeEmptyThread(t, tx, tenant, box))
	row.rawFileID = raw
	id := storeMail(t, tx, tenant, row)
	if err := insertAttachments(t.Context(), tx, tenant, id, []attachmentRow{{
		filename: "quote.pdf", contentType: "application/pdf", size: 11, sha256: hashOf("left behind"),
		fileID: attachment, state: fileStored,
	}}); err != nil {
		t.Fatalf("insertAttachments() error = %v, want nil", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("committing the mail that holds the files: %v", err)
	}
}

// deletedCount returns the count of deleted files the sweep line a bufferLogger wrote names, -1 when it wrote none.
func deletedCount(t *testing.T, written *bytes.Buffer) float64 {
	t.Helper()
	for _, line := range loggedLines(t, written) {
		if line[slog.MessageKey] == "email: swept the files no stored mail holds" {
			return line["deleted"].(float64)
		}
	}
	return -1
}

func TestSweepDeletesTheFilesAStoppedRunLeftBehind(t *testing.T) {
	t.Parallel()

	s, pool, store, written := testSweeper(t, 100)
	left := []uuid.UUID{
		leaveFile(t, pool, store, sdk.DefaultTenantID, "writing"),
		leaveFile(t, pool, store, sdk.DefaultTenantID, "writing"),
	}
	fresh := uuid.Must(uuid.NewV7())
	saveFile(t, store, sdk.DefaultTenantID, fresh, "still being written")
	queueAt(t, pool, sdk.DefaultTenantID, fresh, "writing", 19)

	s.sweep(t.Context())

	for _, id := range left {
		if !fileGone(t, store, sdk.DefaultTenantID, id) {
			t.Errorf("the file %v a stopped run left behind is kept, want it deleted", id)
		}
	}
	if state, _ := queueRow(t.Context(), pool, sdk.DefaultTenantID, fresh); state != "writing" ||
		fileGone(t, store, sdk.DefaultTenantID, fresh) {
		t.Errorf("a file younger than the orphan age is %q, want it left writing and kept", state)
	}
	if rows := queueSize(t, pool, sdk.DefaultTenantID); rows != 1 {
		t.Errorf("the queue holds %d rows, want only the young file's", rows)
	}
	if lines := errorLines(t, written); len(lines) != 0 {
		t.Errorf("the sweep logged %v, want no failure", lines)
	}
}

func TestSweepDeletesEveryQueuedFileAndItsRow(t *testing.T) {
	t.Parallel()

	s, pool, store, _ := testSweeper(t, 100)
	queued := uuid.Must(uuid.NewV7())
	saveFile(t, store, sdk.DefaultTenantID, queued, "queued")
	queueAt(t, pool, sdk.DefaultTenantID, queued, "deleting", 6)

	s.sweep(t.Context())

	if !fileGone(t, store, sdk.DefaultTenantID, queued) || queueSize(t, pool, sdk.DefaultTenantID) != 0 {
		t.Error("the queued file or its row is kept, want both deleted")
	}
}

func TestSweepKeepsTheFilesAStoredMailHoldsAndRemovesTheirRows(t *testing.T) {
	t.Parallel()

	for name, states := range map[string][2]string{
		"a raw copy queued as deleting and an attachment left writing": {"deleting", "writing"},
		"a raw copy left writing and an attachment queued as deleting": {"writing", "deleting"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s, pool, store, written := testSweeper(t, 100)
			raw := leaveFile(t, pool, store, sdk.DefaultTenantID, states[0])
			attachment := leaveFile(t, pool, store, sdk.DefaultTenantID, states[1])
			holdFiles(t, pool, sdk.DefaultTenantID, raw, attachment)
			free := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")

			s.sweep(t.Context())

			for _, id := range []uuid.UUID{raw, attachment} {
				if fileGone(t, store, sdk.DefaultTenantID, id) {
					t.Errorf("the file %v a stored mail holds is gone, want it kept", id)
				}
			}
			if !fileGone(t, store, sdk.DefaultTenantID, free) {
				t.Error("the file no stored mail holds is kept, want it deleted")
			}
			if rows := queueSize(t, pool, sdk.DefaultTenantID); rows != 0 {
				t.Errorf("the queue holds %d rows, want the rows of the held files removed with the rest", rows)
			}
			if deleted := deletedCount(t, written); deleted != 1 {
				t.Errorf("the sweep logged %v files deleted, want only the file no stored mail holds", deleted)
			}
			if lines := errorLines(t, written); len(lines) != 0 {
				t.Errorf("the sweep logged %v, want no failure", lines)
			}
		})
	}
}

func TestSweepDeletesAFileOnlyAMailOfAnotherWorkspaceHolds(t *testing.T) {
	t.Parallel()

	s, pool, store, _ := testSweeper(t, 100)
	acme := storeWorkspace(t, pool)
	raw := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	attachment := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	holdFiles(t, pool, acme, raw, attachment)

	s.sweep(t.Context())

	for _, id := range []uuid.UUID{raw, attachment} {
		if !fileGone(t, store, sdk.DefaultTenantID, id) {
			t.Errorf("the default workspace's file %v is kept, want it deleted when only Acme's mail names its id", id)
		}
	}
	if rows := queueSize(t, pool, sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the queue holds %d rows, want none", rows)
	}
}

func TestSweepLogsTheHoldersOfAFileItCannotCountAndKeepsTheFile(t *testing.T) {
	t.Parallel()

	s, pool, store, written := testSweeper(t, 100)
	id := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE plugin_email.attachments RENAME COLUMN file_id TO kept_file_id"); err != nil {
		t.Fatalf("renaming the file column of the attachments: %v", err)
	}

	s.sweep(t.Context())

	lines := errorLines(t, written)
	if len(lines) != 1 || lines[0]["file"] != id.String() ||
		!strings.Contains(lines[0]["error"].(string), "email: count the holders of a queued file: ") {
		t.Errorf("the sweep logged %v, want the file whose holders it could not count named once", lines)
	}
	if fileGone(t, store, sdk.DefaultTenantID, id) {
		t.Error("the file is gone, want it kept while its holders are not known")
	}
	if state, _ := queueRow(t.Context(), pool, sdk.DefaultTenantID, id); state != "deleting" {
		t.Errorf("the row of the file is %q, want it kept deleting for the next sweep", state)
	}
}

func TestSweepCountsAFileTheStoreNoLongerHoldsAsDeleted(t *testing.T) {
	t.Parallel()

	s, pool, _, written := testSweeper(t, 100)
	queueAt(t, pool, sdk.DefaultTenantID, uuid.Must(uuid.NewV7()), "deleting", 6)

	s.sweep(t.Context())

	if rows := queueSize(t, pool, sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the queue holds %d rows, want the row of a file the store never held removed", rows)
	}
	if lines := errorLines(t, written); len(lines) != 0 {
		t.Errorf("the sweep logged %v, want no failure", lines)
	}
}

func TestSweepDeletesEachFileInTheWorkspaceItsRowNames(t *testing.T) {
	t.Parallel()

	s, pool, store, _ := testSweeper(t, 100)
	acme := storeWorkspace(t, pool)
	id := uuid.Must(uuid.NewV7())
	saveFile(t, store, sdk.DefaultTenantID, id, "the default workspace's file")
	saveFile(t, store, acme, id, "Acme's file")
	queueAt(t, pool, acme, id, "deleting", 11)

	s.sweep(t.Context())

	if !fileGone(t, store, acme, id) {
		t.Error("Acme's queued file is kept, want it deleted in Acme")
	}
	if fileGone(t, store, sdk.DefaultTenantID, id) {
		t.Error("the default workspace's file under the same id is gone, want only Acme's deleted")
	}
	if rows := queueSize(t, pool, acme); rows != 0 {
		t.Errorf("Acme's queue holds %d rows, want none", rows)
	}
}

func TestSweepDrainsAQueueLargerThanOneBatchInOneTick(t *testing.T) {
	t.Parallel()

	s, pool, store, _ := testSweeper(t, 2)
	acme := storeWorkspace(t, pool)
	var kept []uuid.UUID
	for range 5 {
		kept = append(kept, leaveFile(t, pool, store, sdk.DefaultTenantID, "writing"))
	}
	for range 3 {
		kept = append(kept, leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting"))
	}
	acmeFile := leaveFile(t, pool, store, acme, "writing")

	s.sweep(t.Context())

	for _, id := range kept {
		if !fileGone(t, store, sdk.DefaultTenantID, id) {
			t.Errorf("the file %v is kept after one tick, want every batch of the queue drained", id)
		}
	}
	if !fileGone(t, store, acme, acmeFile) {
		t.Error("Acme's file is kept after one tick, want it deleted beside the default workspace's")
	}
	if rows := queueSize(t, pool, sdk.DefaultTenantID) + queueSize(t, pool, acme); rows != 0 {
		t.Errorf("the queue holds %d rows after one tick, want none", rows)
	}
}

func TestMarkLeftBehindTakesOneBatchOfEachWorkspaceAStatement(t *testing.T) {
	t.Parallel()

	s, pool, store, _ := testSweeper(t, 2)
	acme := storeWorkspace(t, pool)
	for range 5 {
		leaveFile(t, pool, store, sdk.DefaultTenantID, "writing")
	}
	leaveFile(t, pool, store, acme, "writing")

	marked, err := s.markLeftBehind(t.Context())

	if err != nil || marked != 3 {
		t.Fatalf("markLeftBehind() = %d, %v, want 2 of the default workspace and Acme's 1", marked, err)
	}
	for tenant, want := range map[uuid.UUID]int{sdk.DefaultTenantID: 2, acme: 1} {
		if rows := countRows(t, pool, `SELECT count(*) FROM plugin_email.pending_files
			WHERE tenant_id = $1 AND state = 'deleting'`, tenant); rows != want {
			t.Errorf("%v holds %d files being deleted, want %d", tenant, rows, want)
		}
	}
}

func TestMarkLeftBehindSkipsALockedRow(t *testing.T) {
	t.Parallel()

	s, pool, store, _ := testSweeper(t, 100)
	locked := leaveFile(t, pool, store, sdk.DefaultTenantID, "writing")
	free := leaveFile(t, pool, store, sdk.DefaultTenantID, "writing")
	holder := beginTx(t, pool)
	if _, err := holder.Exec(t.Context(), `SELECT 1 FROM plugin_email.pending_files
		WHERE tenant_id = $1 AND file_id = $2 FOR UPDATE`, sdk.DefaultTenantID, locked); err != nil {
		t.Fatalf("holding the row as a running ingest does: %v", err)
	}

	marked, err := s.markLeftBehind(t.Context())

	if err != nil || marked != 1 {
		t.Fatalf("markLeftBehind() = %d, %v, want only the free row", marked, err)
	}
	for id, want := range map[uuid.UUID]string{locked: "writing", free: "deleting"} {
		if state, _ := queueRow(t.Context(), pool, sdk.DefaultTenantID, id); state != want {
			t.Errorf("the row of %v is %q, want %q", id, state, want)
		}
	}
}

func TestReadQueueGivesEachWorkspaceItsTurnInTheFirstRound(t *testing.T) {
	t.Parallel()

	s, pool, store, _ := testSweeper(t, 2)
	acme := storeWorkspace(t, pool)
	for range 3 {
		leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	}
	leaveFile(t, pool, store, acme, "deleting")

	queued, err := s.readQueue(t.Context(), nil)

	if err != nil || len(queued) != 3 {
		t.Fatalf("readQueue() = %v, %v, want one batch of 2 of the default workspace and Acme's 1", queued, err)
	}
	if queued[0].tenant == queued[1].tenant {
		t.Errorf("readQueue() = %v, want both workspaces in the first round", queued)
	}
	if queued[2].tenant != sdk.DefaultTenantID {
		t.Errorf("readQueue() = %v, want the default workspace's second file last", queued)
	}
}

func TestReadQueueLeavesOutTheFilesThatFailedThisTick(t *testing.T) {
	t.Parallel()

	s, pool, store, _ := testSweeper(t, 100)
	failed := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	other := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")

	queued, err := s.readQueue(t.Context(), []uuid.UUID{failed})

	if want := []queuedFile{{tenant: sdk.DefaultTenantID, id: other}}; err != nil || !reflect.DeepEqual(queued, want) {
		t.Errorf("readQueue() = %v, %v, want only %v", queued, err, want)
	}
}

func TestSweepLogsAFailedDeleteAndTriesItAgainNextTick(t *testing.T) {
	t.Parallel()

	s, pool, store, written := testSweeper(t, 100)
	refused := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	other := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	s.store = deletingStore{FileStore: store, before: func(_ context.Context, id uuid.UUID) error {
		if id == refused {
			return errStoreDown
		}
		return nil
	}}

	s.sweep(t.Context())

	if state, _ := queueRow(t.Context(), pool, sdk.DefaultTenantID, refused); state != "deleting" ||
		fileGone(t, store, sdk.DefaultTenantID, refused) {
		t.Errorf("the refused file's row is %q, want it kept deleting beside its file", state)
	}
	if !fileGone(t, store, sdk.DefaultTenantID, other) {
		t.Error("the other file is kept, want the sweep to go on past a failed delete")
	}
	lines := errorLines(t, written)
	if len(lines) != 1 {
		t.Fatalf("the sweep logged %v, want one failure", lines)
	}
	if lines[0]["workspace"] != sdk.DefaultTenantID.String() || lines[0]["file"] != refused.String() ||
		!strings.Contains(lines[0]["error"].(string), errStoreDown.Error()) {
		t.Errorf("the failure line = %v, want the workspace, the file and the error", lines[0])
	}
	if strings.Contains(written.String(), "@") {
		t.Errorf("the log %q names an address, want ids and reasons only", written)
	}

	s.store = store
	s.sweep(t.Context())

	if !fileGone(t, store, sdk.DefaultTenantID, refused) || queueSize(t, pool, sdk.DefaultTenantID) != 0 {
		t.Error("the refused file is kept after the next tick, want it deleted once the store answers")
	}
}

func TestSweepLogsAQueueItCannotRemoveARowFrom(t *testing.T) {
	t.Parallel()

	s, pool, store, written := testSweeper(t, 100)
	id := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	if _, err := pool.Exec(t.Context(), `CREATE FUNCTION plugin_email.keep_rows() RETURNS trigger LANGUAGE plpgsql
		AS $$ BEGIN RAISE EXCEPTION 'rows are kept'; END $$`); err != nil {
		t.Fatalf("creating the refusing function: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `CREATE TRIGGER keep_rows BEFORE DELETE ON plugin_email.pending_files
		FOR EACH ROW EXECUTE FUNCTION plugin_email.keep_rows()`); err != nil {
		t.Fatalf("refusing every row delete: %v", err)
	}

	s.sweep(t.Context())

	lines := errorLines(t, written)
	if len(lines) != 1 || lines[0]["file"] != id.String() ||
		!strings.Contains(lines[0]["error"].(string), "email: remove a file from the queue: ") {
		t.Errorf("the sweep logged %v, want the row it could not remove named once", lines)
	}
	if !fileGone(t, store, sdk.DefaultTenantID, id) {
		t.Error("the file is kept, want it deleted before its row")
	}
}

func TestSweepLogsEachJobThatCannotReadTheQueue(t *testing.T) {
	t.Parallel()

	pool, _ := newUnmigratedPool(t)
	logger, written := bufferLogger()
	s := &sweeper{pool: pool, logger: logger, pace: sweepSettings{batch: 1, budget: time.Minute, orphanAge: orphanAge}}

	s.sweep(t.Context())

	lines := errorLines(t, written)
	if len(lines) != 2 {
		t.Fatalf("the sweep logged %v, want one failure for each job", lines)
	}
	for i, prefix := range []string{"email: mark the files left behind: ", "email: read the file queue: "} {
		if !strings.HasPrefix(lines[i]["error"].(string), prefix) {
			t.Errorf("failure %d = %v, want it to name %q", i, lines[i], prefix)
		}
	}
}

func TestTheBudgetEndsATick(t *testing.T) {
	t.Parallel()

	s, pool, store, written := testSweeper(t, 100)
	first := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	second := leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	deletes := 0
	s.store = deletingStore{FileStore: store, before: func(ctx context.Context, _ uuid.UUID) error {
		deletes++
		<-ctx.Done()
		return ctx.Err()
	}}
	s.pace.budget = 200 * time.Millisecond

	s.sweep(t.Context())

	if deletes != 1 {
		t.Errorf("the tick tried %d deletes, want it to end at the budget after the first", deletes)
	}
	for _, id := range []uuid.UUID{first, second} {
		if state, _ := queueRow(t.Context(), pool, sdk.DefaultTenantID, id); state != "deleting" {
			t.Errorf("the row of %v is %q, want it kept for the next tick", id, state)
		}
	}
	if lines := errorLines(t, written); len(lines) != 0 {
		t.Errorf("the sweep logged %v, want no failure for a tick its budget ended", lines)
	}
}

func TestStopWithoutStartAnswersNil(t *testing.T) {
	t.Parallel()

	if err := (&sweeper{}).Stop(t.Context()); err != nil {
		t.Errorf("Stop() before Start error = %v, want nil", err)
	}
}

func TestStartRunsNoSweepBeforeTheFirstTick(t *testing.T) {
	t.Parallel()

	s := &sweeper{pace: sweepSettings{interval: time.Hour}}

	s.Start()

	if err := s.Stop(t.Context()); err != nil {
		t.Errorf("Stop() error = %v, want nil after no sweep touched the missing pool", err)
	}
}

func TestTheFirstSweepRunsOneIntervalAfterStart(t *testing.T) {
	t.Parallel()

	s, pool, store, _ := testSweeper(t, 100)
	id := leaveFile(t, pool, store, sdk.DefaultTenantID, "writing")
	s.pace.interval = 100 * time.Millisecond

	s.Start()
	defer func() { _ = s.Stop(context.Background()) }()

	deadline := time.Now().Add(sweepWait)
	for queueSize(t, pool, sdk.DefaultTenantID) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("the file left behind is still queued after %v, want the first tick to sweep it", sweepWait)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !fileGone(t, store, sdk.DefaultTenantID, id) {
		t.Error("the file left behind is kept, want the first tick to delete it")
	}
}

// heldSweeper returns a started sweeper whose one delete is running and waits for release whatever its context says.
func heldSweeper(t *testing.T) (*sweeper, chan struct{}) {
	t.Helper()
	s, pool, store, _ := testSweeper(t, 100)
	leaveFile(t, pool, store, sdk.DefaultTenantID, "deleting")
	entered, release := make(chan struct{}), make(chan struct{})
	s.store = deletingStore{FileStore: store, before: func(context.Context, uuid.UUID) error {
		close(entered)
		<-release
		return nil
	}}
	s.pace.interval = 10 * time.Millisecond
	s.Start()
	select {
	case <-entered:
	case <-time.After(sweepWait):
		t.Fatalf("no sweep reached the delete within %v", sweepWait)
	}
	return s, release
}

func TestStopWaitsForARunningSweep(t *testing.T) {
	t.Parallel()

	s, release := heldSweeper(t)
	stopped := make(chan error, 1)

	go func() { stopped <- s.Stop(context.Background()) }()

	select {
	case err := <-stopped:
		t.Fatalf("Stop() = %v while a sweep runs, want it to wait for the sweep", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Stop() after the sweep ended error = %v, want nil", err)
		}
	case <-time.After(sweepWait):
		t.Fatalf("Stop() is still waiting %v after the sweep ended", sweepWait)
	}
}

func TestStopAnswersTheErrorOfItsContextWhenASweepOverrunsIt(t *testing.T) {
	t.Parallel()

	s, release := heldSweeper(t)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	err := s.Stop(ctx)

	close(release)
	<-s.done
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop() while a sweep overruns it error = %v, want %v", err, context.DeadlineExceeded)
	}
}

// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob/memblob"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

// fileCap is the most bytes one file holds in the host's file store of the file tests.
const fileCap = 1 << 20

// errStoreDown is the failure of a file store that cannot be reached.
var errStoreDown = errors.New("file store unreachable")

// savedCall is one SaveAs call a recordingStore saw, beside the state its id's queue row held at that moment.
type savedCall struct {
	// id is the id the file was saved under.
	id uuid.UUID
	// contentType is the media type the file was sent with.
	contentType string
	// queued is the state of the file's queue row during the call, empty when it held none.
	queued string
}

// recordingStore hands every call to the host's file store, recording each SaveAs and failing the one at failAt.
type recordingStore struct {
	sdk.FileStore
	// pool reads the queue row of each file being saved.
	pool *pgxpool.Pool
	// failAt counts from 1 the SaveAs call that answers errStoreDown, 0 for none.
	failAt int
	// mu guards saves.
	mu sync.Mutex
	// saves lists every SaveAs call in order.
	saves []savedCall
}

// SaveAs records the call and the queue row of id, then saves body through the host's store unless the call fails.
func (r *recordingStore) SaveAs(ctx context.Context, id uuid.UUID, contentType string, body io.Reader) (
	sdk.StoredFile, error) {
	state, _ := queueRow(ctx, r.pool, sdk.TenantOrDefault(ctx), id)
	r.mu.Lock()
	r.saves = append(r.saves, savedCall{id: id, contentType: contentType, queued: state})
	calls := len(r.saves)
	r.mu.Unlock()
	if calls == r.failAt {
		return sdk.StoredFile{}, errStoreDown
	}
	return r.FileStore.SaveAs(ctx, id, contentType, body)
}

// queueRow returns the state and the size of the queue row of the file id in the workspace tenant, empty for none.
func queueRow(ctx context.Context, db rowReader, tenant, id uuid.UUID) (string, int64) {
	var state string
	var size int64
	_ = db.QueryRow(ctx, "SELECT state, size FROM plugin_email.pending_files WHERE tenant_id = $1 AND file_id = $2",
		tenant, id).Scan(&state, &size)
	return state, size
}

// newMailFiles returns the files of mail over a migrated database and the host's file store over memory, beside both.
func newMailFiles(t *testing.T, maxBytes int64) (mailFiles, *pgxpool.Pool, *recordingStore) {
	t.Helper()
	pool := newMigratedPool(t)
	store := &recordingStore{FileStore: storage.NewFileStore(memblob.OpenBucket(nil), "email", maxBytes), pool: pool}
	return mailFiles{
		pool:           pool,
		store:          store,
		settings:       postgres.NewSettingStore(pool, nil).ForPlugin("email"),
		logger:         slog.New(slog.DiscardHandler),
		caps:           fileCaps{attachmentMaxBytes: 8, workspaceMaxBytes: 100},
		cleanupTimeout: 10 * time.Second,
	}, pool, store
}

// storedBytes returns the bytes the host's store keeps under id in the workspace tenant, failing when it keeps none.
func storedBytes(t *testing.T, store sdk.FileStore, tenant, id uuid.UUID) []byte {
	t.Helper()
	file, err := store.Open(sdk.WithTenant(t.Context(), tenant), id)
	if err != nil {
		t.Fatalf("opening the file %v: %v", id, err)
	}
	defer func() { _ = file.Close() }()
	held, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("reading the file %v: %v", id, err)
	}
	return held
}

// hashOf returns the SHA-256 of content.
func hashOf(content string) []byte {
	sum := sha256.Sum256([]byte(content))
	return sum[:]
}

// keepRun keeps files in the default workspace and fails the test when keepFiles fails.
func keepRun(t *testing.T, files mailFiles, state string, mail ...mailFile) []keptFile {
	t.Helper()
	kept, err := files.keepFiles(t.Context(), sdk.DefaultTenantID, mail, state)
	if err != nil {
		t.Fatalf("keepFiles() error = %v, want nil", err)
	}
	return kept
}

// queueAt stores one queue row of the file id in the workspace tenant in state.
func queueAt(t *testing.T, db execer, tenant, id uuid.UUID, state string, size int64) {
	t.Helper()
	if _, err := db.Exec(t.Context(), `INSERT INTO plugin_email.pending_files (tenant_id, file_id, state, size)
		VALUES ($1, $2, $3, $4)`, tenant, id, state, size); err != nil {
		t.Fatalf("queueing the file %v: %v", id, err)
	}
}

func TestMigrateCreatesThePendingFilesTable(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	if !tableExists(t, pool, "plugin_email.pending_files") {
		t.Error("migrate() left out plugin_email.pending_files, want it created")
	}
}

func TestThePendingFilesTableTakesTheTwoQueueStatesOnly(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	_, err := pool.Exec(t.Context(), `INSERT INTO plugin_email.pending_files (tenant_id, file_id, state, size)
		VALUES ($1, $2, 'deleted', 1)`, sdk.DefaultTenantID, uuid.Must(uuid.NewV7()))

	if code := pgCode(err); code != checkViolation {
		t.Errorf("a queue row in the state deleted error = %v, want the check to refuse it", err)
	}
}

func TestFilesOfListsTheRawCopyThenEveryAttachmentAndInlineImage(t *testing.T) {
	t.Parallel()

	files := mailFiles{caps: fileCaps{attachmentMaxBytes: 8}}
	m := &message.Message{
		Raw:          []byte("raw mail"),
		Attachments:  []message.Attachment{{ContentType: "application/pdf", Content: []byte("12345678")}},
		InlineImages: []message.Attachment{{ContentType: "image/png", Content: []byte("123456789")}},
	}

	for _, rawWhole := range []bool{true, false} {
		got := files.filesOf(m, rawWhole)

		want := []mailFile{
			{contentType: rawMailType, content: []byte("raw mail"), tooLarge: !rawWhole},
			{contentType: "application/pdf", content: []byte("12345678")},
			{contentType: "image/png", content: []byte("123456789"), tooLarge: true},
		}
		if len(got) != len(want) {
			t.Fatalf("filesOf(rawWhole=%t) = %+v, want %+v", rawWhole, got, want)
		}
		for i := range want {
			if got[i].contentType != want[i].contentType || !bytes.Equal(got[i].content, want[i].content) ||
				got[i].tooLarge != want[i].tooLarge {
				t.Errorf("filesOf(rawWhole=%t)[%d] = %+v, want %+v", rawWhole, i, got[i], want[i])
			}
		}
	}
}

func TestKeepFilesSavesTheRawCopyAsAMailAndEachAttachmentUnderItsType(t *testing.T) {
	t.Parallel()

	files, _, store := newMailFiles(t, fileCap)

	kept := keepRun(t, files, fileStored,
		mailFile{contentType: rawMailType, content: []byte("raw mail")},
		mailFile{contentType: "application/pdf", content: []byte("quote")})

	for i, want := range []struct {
		contentType string
		content     string
	}{{rawMailType, "raw mail"}, {"application/pdf", "quote"}} {
		file := kept[i]
		if file.state != fileStored || file.fileID() == uuid.Nil || file.fileID().Version() != 7 {
			t.Errorf("file %d = %+v, want it stored under a new UUIDv7", i, file)
		}
		if file.contentType != want.contentType || file.size != int64(len(want.content)) ||
			!bytes.Equal(file.sha256, hashOf(want.content)) {
			t.Errorf("file %d = %+v, want %s of %d bytes and their hash", i, file, want.contentType, len(want.content))
		}
		if held := storedBytes(t, store, sdk.DefaultTenantID, file.fileID()); string(held) != want.content {
			t.Errorf("the store keeps %q under file %d, want %q", held, i, want.content)
		}
	}
}

func TestKeepFilesSavesATypeTheStoreWouldRefuseAsAnOctetStream(t *testing.T) {
	t.Parallel()

	for _, contentType := range []string{"inline", "", "not a type"} {
		t.Run(contentType, func(t *testing.T) {
			t.Parallel()
			files, _, store := newMailFiles(t, fileCap)

			kept := keepRun(t, files, fileStored, mailFile{contentType: contentType, content: []byte("notes")})

			if kept[0].state != fileStored || kept[0].contentType != octetStream {
				t.Errorf("the file of type %q = %+v, want it stored as %s", contentType, kept[0], octetStream)
			}
			if sent := store.saves[0].contentType; sent != octetStream {
				t.Errorf("the file of type %q went to the store as %q, want %s", contentType, sent, octetStream)
			}
		})
	}
}

func TestKeepFilesQueuesEveryFileAsWritingBeforeItsBytes(t *testing.T) {
	t.Parallel()

	files, pool, store := newMailFiles(t, fileCap)

	kept := keepRun(t, files, fileStored,
		mailFile{contentType: rawMailType, content: []byte("raw mail")},
		mailFile{contentType: "application/pdf", content: []byte("quote")})

	if len(store.saves) != 2 {
		t.Fatalf("the store saw %d saves, want 2", len(store.saves))
	}
	if slices.ContainsFunc(store.saves, func(call savedCall) bool { return call.queued != "writing" }) {
		t.Errorf("the saves found their queue rows as %+v, want every row written as writing first", store.saves)
	}
	for _, file := range kept {
		if state, size := queueRow(t.Context(), pool, sdk.DefaultTenantID, file.fileID()); state != "writing" ||
			size != file.size {
			t.Errorf("the queue row of %v = %q of %d bytes, want writing of %d", file.fileID(), state, size, file.size)
		}
	}
}

func TestKeepFilesSavesEachFileInTheWorkspaceItServes(t *testing.T) {
	t.Parallel()

	files, pool, store := newMailFiles(t, fileCap)
	acme := storeWorkspace(t, pool)

	kept, err := files.keepFiles(t.Context(), acme, []mailFile{{contentType: "text/plain", content: []byte("notes")}},
		fileStored)

	if err != nil {
		t.Fatalf("keepFiles() error = %v, want nil", err)
	}
	if held := storedBytes(t, store, acme, kept[0].fileID()); string(held) != "notes" {
		t.Errorf("Acme keeps %q, want the file it was given", held)
	}
	if _, err := store.Open(t.Context(), kept[0].fileID()); !errors.Is(err, sdk.ErrFileNotFound) {
		t.Errorf("opening the file in the default workspace error = %v, want %v", err, sdk.ErrFileNotFound)
	}
	if state, _ := queueRow(t.Context(), pool, acme, kept[0].fileID()); state != "writing" {
		t.Errorf("the queue row in Acme = %q, want writing", state)
	}
}

func TestKeepFilesMarksAFileTheStoreRefusesAsTooLargeAndGoesOn(t *testing.T) {
	t.Parallel()

	files, pool, store := newMailFiles(t, 6)

	kept := keepRun(t, files, fileStored,
		mailFile{contentType: rawMailType, content: []byte("raw mail over the cap")},
		mailFile{contentType: "application/pdf", content: []byte("quote")})

	if kept[0].state != fileTooLarge || kept[0].fileID() != uuid.Nil || kept[0].size != 21 {
		t.Errorf("the raw copy = %+v, want it too large with no file and its size kept", kept[0])
	}
	if state, _ := queueRow(t.Context(), pool, sdk.DefaultTenantID, kept[0].queueID); state != "writing" {
		t.Errorf("the refused file's queue row = %q, want it kept for the run to clear", state)
	}
	if kept[1].state != fileStored || string(storedBytes(t, store, sdk.DefaultTenantID, kept[1].fileID())) != "quote" {
		t.Errorf("the attachment = %+v, want it stored after the refused raw copy", kept[1])
	}
}

func TestKeepFilesSendsNoFileThatIsTooLargeOrOverTheLimit(t *testing.T) {
	t.Parallel()

	for _, state := range []string{fileStored, fileOverLimit} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			files, pool, store := newMailFiles(t, fileCap)

			kept := keepRun(t, files, state, mailFile{contentType: "image/png", content: []byte("photo"), tooLarge: true})

			if kept[0].state != fileTooLarge || kept[0].queueID != uuid.Nil || kept[0].contentType != "image/png" ||
				kept[0].size != 5 || !bytes.Equal(kept[0].sha256, hashOf("photo")) {
				t.Errorf("the file over its cap = %+v, want it too large, named, sized and hashed with no queue row", kept[0])
			}
			if len(store.saves) != 0 {
				t.Errorf("the store saw %d saves, want none", len(store.saves))
			}
			if rows := countRows(t, pool, "SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1",
				sdk.DefaultTenantID); rows != 0 {
				t.Errorf("the queue holds %d rows, want none", rows)
			}
		})
	}
}

func TestKeepFilesSendsNoFileOverTheWorkspaceLimit(t *testing.T) {
	t.Parallel()

	files, pool, store := newMailFiles(t, fileCap)

	kept := keepRun(t, files, fileOverLimit,
		mailFile{contentType: rawMailType, content: []byte("raw mail")},
		mailFile{contentType: "application/pdf", content: []byte("quote")})

	for i, file := range kept {
		if file.state != fileOverLimit || file.queueID != uuid.Nil || file.size == 0 || len(file.sha256) != 32 {
			t.Errorf("file %d = %+v, want it over the limit, sized and hashed with no queue row", i, file)
		}
	}
	if len(store.saves) != 0 {
		t.Errorf("the store saw %d saves, want none", len(store.saves))
	}
	if rows := countRows(t, pool, "SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1",
		sdk.DefaultTenantID); rows != 0 {
		t.Errorf("the queue holds %d rows, want none", rows)
	}
}

func TestKeepFilesQueuesTheWholeRunForDeletionWhenASaveFails(t *testing.T) {
	t.Parallel()

	files, pool, store := newMailFiles(t, fileCap)
	store.failAt = 2

	kept, err := files.keepFiles(t.Context(), sdk.DefaultTenantID, []mailFile{
		{contentType: rawMailType, content: []byte("raw mail")},
		{contentType: "application/pdf", content: []byte("quote")},
		{contentType: "image/png", content: []byte("photo")},
	}, fileStored)

	if !errors.Is(err, errFileStore) || !errors.Is(err, errStoreDown) || kept != nil {
		t.Fatalf("keepFiles() = %v, %v, want no files and the file store failure", kept, err)
	}
	if rows := countRows(t, pool, `SELECT count(*) FROM plugin_email.pending_files
		WHERE tenant_id = $1 AND state = 'deleting'`, sdk.DefaultTenantID); rows != 3 {
		t.Errorf("the queue holds %d files being deleted, want all 3 files of the run", rows)
	}
}

func TestKeepFilesReportsAQueueItCannotWrite(t *testing.T) {
	t.Parallel()

	files, _, store := newMailFiles(t, fileCap)

	_, err := files.keepFiles(t.Context(), uuid.Must(uuid.NewV7()), []mailFile{{contentType: "text/plain"}}, fileStored)

	if code := pgCode(err); code != foreignKeyViolation || !strings.HasPrefix(err.Error(), "email: queue the files: ") {
		t.Errorf("keepFiles() in a workspace that does not exist error = %v, want the queue write named", err)
	}
	if len(store.saves) != 0 {
		t.Errorf("the store saw %d saves, want none before the queue holds the files", len(store.saves))
	}
}

func TestKeepFilesReportsAnIDItCannotMake(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	_, err := mailFiles{}.keepFiles(t.Context(), sdk.DefaultTenantID, []mailFile{{contentType: "text/plain"}},
		fileStored)

	if !errors.Is(err, errEntropy) {
		t.Errorf("keepFiles() error = %v, want %v", err, errEntropy)
	}
}

func TestQueueFilesForDeletionMarksEveryQueuedFileOfARun(t *testing.T) {
	t.Parallel()

	files, pool, _ := newMailFiles(t, fileCap)
	kept := keepRun(t, files, fileStored,
		mailFile{contentType: rawMailType, content: []byte("raw mail")},
		mailFile{contentType: "image/png", content: []byte("photo over the cap"), tooLarge: true},
		mailFile{contentType: "application/pdf", content: []byte("quote")})
	other := uuid.Must(uuid.NewV7())
	queueAt(t, pool, sdk.DefaultTenantID, other, "writing", 3)

	if err := files.queueFilesForDeletion(t.Context(), sdk.DefaultTenantID, kept); err != nil {
		t.Fatalf("queueFilesForDeletion() error = %v, want nil", err)
	}

	for _, file := range []keptFile{kept[0], kept[2]} {
		if state, size := queueRow(t.Context(), pool, sdk.DefaultTenantID, file.queueID); state != "deleting" ||
			size != file.size {
			t.Errorf("the queue row of %v = %q of %d bytes, want deleting of %d", file.queueID, state, size, file.size)
		}
	}
	if state, _ := queueRow(t.Context(), pool, sdk.DefaultTenantID, other); state != "writing" {
		t.Errorf("another run's queue row = %q, want it left writing", state)
	}
	if rows := countRows(t, pool, "SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1",
		sdk.DefaultTenantID); rows != 3 {
		t.Errorf("the queue holds %d rows, want the 2 of the run and the other run's 1", rows)
	}
}

func TestQueueFilesForDeletionRunsUnderACancelledCaller(t *testing.T) {
	t.Parallel()

	files, pool, _ := newMailFiles(t, fileCap)
	kept := keepRun(t, files, fileStored, mailFile{contentType: rawMailType, content: []byte("raw mail")})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := files.queueFilesForDeletion(ctx, sdk.DefaultTenantID, kept); err != nil {
		t.Fatalf("queueFilesForDeletion() under a cancelled caller error = %v, want nil", err)
	}

	if state, _ := queueRow(t.Context(), pool, sdk.DefaultTenantID, kept[0].queueID); state != "deleting" {
		t.Errorf("the queue row = %q, want deleting", state)
	}
}

func TestQueueFilesForDeletionRestoresARowTheSweepRemoved(t *testing.T) {
	t.Parallel()

	files, pool, _ := newMailFiles(t, fileCap)
	kept := keepRun(t, files, fileStored, mailFile{contentType: rawMailType, content: []byte("raw mail")})
	if _, err := pool.Exec(t.Context(), "DELETE FROM plugin_email.pending_files WHERE tenant_id = $1",
		sdk.DefaultTenantID); err != nil {
		t.Fatalf("removing the queue row as the sweep does: %v", err)
	}

	if err := files.queueFilesForDeletion(t.Context(), sdk.DefaultTenantID, kept); err != nil {
		t.Fatalf("queueFilesForDeletion() error = %v, want nil", err)
	}

	if state, size := queueRow(t.Context(), pool, sdk.DefaultTenantID, kept[0].queueID); state != "deleting" ||
		size != 8 {
		t.Errorf("the restored queue row = %q of %d bytes, want deleting of 8", state, size)
	}
}

func TestQueueFilesForDeletionStopsAtTheCleanupTimeout(t *testing.T) {
	t.Parallel()

	files, pool, _ := newMailFiles(t, fileCap)
	kept := keepRun(t, files, fileStored, mailFile{contentType: rawMailType, content: []byte("raw mail")})
	holder := beginTx(t, pool)
	if _, err := holder.Exec(t.Context(), "SELECT 1 FROM plugin_email.pending_files WHERE tenant_id = $1 FOR UPDATE",
		sdk.DefaultTenantID); err != nil {
		t.Fatalf("holding the queue row: %v", err)
	}
	files.cleanupTimeout = 200 * time.Millisecond

	err := files.queueFilesForDeletion(t.Context(), sdk.DefaultTenantID, kept)

	if !errors.Is(err, context.DeadlineExceeded) ||
		!strings.HasPrefix(err.Error(), "email: queue the files for deletion: ") {
		t.Errorf("queueFilesForDeletion() past the clean up timeout error = %v, want %v named", err,
			context.DeadlineExceeded)
	}
}

func TestCheckWorkspaceLimitCountsTheUsedAndTheQueuedBytes(t *testing.T) {
	t.Parallel()

	files, pool, _ := newMailFiles(t, fileCap)
	addBytes(t, pool, sdk.DefaultTenantID, 30)
	queueAt(t, pool, sdk.DefaultTenantID, uuid.Must(uuid.NewV7()), "writing", 20)
	queueAt(t, pool, sdk.DefaultTenantID, uuid.Must(uuid.NewV7()), "deleting", 10)
	acme := storeWorkspace(t, pool)
	queueAt(t, pool, acme, uuid.Must(uuid.NewV7()), "writing", 90)

	for size, want := range map[int64]string{40: fileStored, 41: fileOverLimit} {
		state, err := files.checkWorkspaceLimit(t.Context(), sdk.DefaultTenantID, size)

		if err != nil || state != want {
			t.Errorf("checkWorkspaceLimit(%d) over 60 held of 100 = %q, %v, want %q", size, state, err, want)
		}
	}
}

func TestCheckWorkspaceLimitTakesTheWorkspacesOwnLimit(t *testing.T) {
	t.Parallel()

	files, pool, _ := newMailFiles(t, fileCap)
	acme := storeWorkspace(t, pool)
	holdLimit(t, files.settings, acme, "10")

	for tenant, want := range map[uuid.UUID]string{acme: fileOverLimit, sdk.DefaultTenantID: fileStored} {
		state, err := files.checkWorkspaceLimit(t.Context(), tenant, 20)

		if err != nil || state != want {
			t.Errorf("checkWorkspaceLimit(20) in %v = %q, %v, want %q", tenant, state, err, want)
		}
	}
}

func TestCheckWorkspaceLimitTakesZeroAsNoLimitAndReadsNoUsage(t *testing.T) {
	t.Parallel()

	files, _, _ := newMailFiles(t, fileCap)
	files.caps.workspaceMaxBytes = 0
	files.pool = nil

	state, err := files.checkWorkspaceLimit(t.Context(), sdk.DefaultTenantID, 1<<40)

	if err != nil || state != fileStored {
		t.Errorf("checkWorkspaceLimit() with no limit = %q, %v, want %q", state, err, fileStored)
	}
}

func TestCheckWorkspaceLimitReportsALimitItCannotRead(t *testing.T) {
	t.Parallel()

	files, _, _ := newMailFiles(t, fileCap)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := files.checkWorkspaceLimit(ctx, sdk.DefaultTenantID, 1)

	if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), "email: read the workspace mail limit: ") {
		t.Errorf("checkWorkspaceLimit() error = %v, want the failed limit read", err)
	}
}

func TestCheckWorkspaceLimitReportsAUsageItCannotRead(t *testing.T) {
	t.Parallel()

	pool, _ := newUnmigratedPool(t)
	files := mailFiles{
		pool:     pool,
		settings: postgres.NewSettingStore(pool, nil).ForPlugin("email"),
		logger:   slog.New(slog.DiscardHandler),
		caps:     fileCaps{workspaceMaxBytes: 100},
	}

	_, err := files.checkWorkspaceLimit(t.Context(), sdk.DefaultTenantID, 1)

	if err == nil || !strings.HasPrefix(err.Error(), "email: read the bytes a workspace holds: ") {
		t.Errorf("checkWorkspaceLimit() on a database with no email schema error = %v, want the usage read named", err)
	}
}

func TestDequeueFilesRemovesTheWritingRowsOfARun(t *testing.T) {
	t.Parallel()

	files, pool, _ := newMailFiles(t, fileCap)
	kept := keepRun(t, files, fileStored,
		mailFile{contentType: rawMailType, content: []byte("raw mail")},
		mailFile{contentType: "image/png", content: []byte("photo over the cap"), tooLarge: true},
		mailFile{contentType: "application/pdf", content: []byte("quote")})
	other := uuid.Must(uuid.NewV7())
	queueAt(t, pool, sdk.DefaultTenantID, other, "writing", 3)
	tx := beginTx(t, pool)

	if err := dequeueFiles(t.Context(), tx, sdk.DefaultTenantID, kept); err != nil {
		t.Fatalf("dequeueFiles() error = %v, want nil", err)
	}

	if rows := countRows(t, tx, "SELECT count(*) FROM plugin_email.pending_files WHERE tenant_id = $1",
		sdk.DefaultTenantID); rows != 1 {
		t.Errorf("the queue holds %d rows, want only the other run's", rows)
	}
	if state, _ := queueRow(t.Context(), tx, sdk.DefaultTenantID, other); state != "writing" {
		t.Errorf("another run's queue row = %q, want it left writing", state)
	}
}

func TestDequeueFilesAnswersSweptWhenTheSweepTookAFile(t *testing.T) {
	t.Parallel()

	for name, took := range map[string]string{
		"marked for deletion": "UPDATE plugin_email.pending_files SET state = 'deleting' WHERE file_id = $1",
		"already removed":     "DELETE FROM plugin_email.pending_files WHERE file_id = $1",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files, pool, _ := newMailFiles(t, fileCap)
			kept := keepRun(t, files, fileStored,
				mailFile{contentType: rawMailType, content: []byte("raw mail")},
				mailFile{contentType: "application/pdf", content: []byte("quote")})
			if _, err := pool.Exec(t.Context(), took, kept[1].queueID); err != nil {
				t.Fatalf("taking a file as the sweep does: %v", err)
			}

			err := dequeueFiles(t.Context(), beginTx(t, pool), sdk.DefaultTenantID, kept)

			if !errors.Is(err, errFilesSwept) {
				t.Errorf("dequeueFiles() error = %v, want %v", err, errFilesSwept)
			}
		})
	}
}

func TestFileStatementsReportAnEndedTransaction(t *testing.T) {
	t.Parallel()

	tx := endedTx(t)
	kept := []keptFile{{queueID: uuid.Must(uuid.NewV7()), size: 1}}
	for prefix, call := range map[string]func() error{
		"email: queue the files for deletion: ": func() error {
			return markDeleting(t.Context(), tx, sdk.DefaultTenantID, kept)
		},
		"email: dequeue the files: ": func() error { return dequeueFiles(t.Context(), tx, sdk.DefaultTenantID, kept) },
	} {
		err := call()

		if !errors.Is(err, pgx.ErrTxClosed) || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("a statement on an ended transaction error = %v, want %q naming %v", err, prefix, pgx.ErrTxClosed)
		}
	}
}

func TestKeptFileNamesItsFileOnlyWhenTheStoreKeepsIt(t *testing.T) {
	t.Parallel()

	id := uuid.Must(uuid.NewV7())
	for state, want := range map[string]uuid.UUID{fileStored: id, fileTooLarge: uuid.Nil, fileOverLimit: uuid.Nil} {
		if got := (keptFile{queueID: id, state: state}).fileID(); got != want {
			t.Errorf("fileID() of a %s file = %v, want %v", state, got, want)
		}
	}
}

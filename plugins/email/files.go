// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

// rawMailType is the media type a raw mail is kept under.
const rawMailType = "message/rfc822"

// octetStream is the media type of a file whose own type the file store would refuse.
const octetStream = "application/octet-stream"

// errFileStore reports a file the file store could not save.
var errFileStore = errors.New("email: the file store failed")

// errFilesSwept reports a run whose files the sweep took before the run could tie them to its mail.
var errFilesSwept = errors.New("email: the sweep took the files of the run")

// statementRunner runs one statement on a pool or inside a transaction.
type statementRunner interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// mailFile is one file of a mail on its way to the file store.
type mailFile struct {
	// contentType is the media type the mail gives the file.
	contentType string
	// content is the bytes of the file.
	content []byte
	// tooLarge reports a file over the cap of its own kind, which is never sent.
	tooLarge bool
}

// keptFile is what the file store keeps of one file of a mail, or why it keeps nothing.
type keptFile struct {
	// queueID names the file's row in pending_files, uuid.Nil when the file never went to the store.
	queueID uuid.UUID
	// state says whether the store keeps the file, and why not when it does not.
	state string
	// contentType is the media type the store keeps the file under, or the one it would take.
	contentType string
	// size is the size of the file.
	size int64
	// sha256 is the hash of the file.
	sha256 []byte
}

// fileID returns the id the store keeps the file under, uuid.Nil when it keeps none.
func (k keptFile) fileID() uuid.UUID {
	if k.state != fileStored {
		return uuid.Nil
	}
	return k.queueID
}

// mailFiles keeps the files of mail in the file store, each one queued in pending_files before its bytes.
type mailFiles struct {
	// pool reaches the queue and the bytes each workspace holds.
	pool *pgxpool.Pool
	// store keeps the bytes of each file.
	store sdk.FileStore
	// settings holds a workspace's own mail limit.
	settings sdk.SettingStore
	// logger records a workspace's own limit that does not read.
	logger *slog.Logger
	// caps bound the bytes one attachment and one workspace keep.
	caps fileCaps
	// cleanupTimeout bounds the queueing of a failed run's files for deletion.
	cleanupTimeout time.Duration
}

// filesOf returns the raw copy of m, too large unless rawWhole, then each attachment and inline image of m in order.
func (f mailFiles) filesOf(m *message.Message, rawWhole bool) []mailFile {
	files := []mailFile{{contentType: rawMailType, content: m.Raw, tooLarge: !rawWhole}}
	for _, part := range slices.Concat(m.Attachments, m.InlineImages) {
		files = append(files, mailFile{
			contentType: part.ContentType, content: part.Content, tooLarge: len(part.Content) > f.caps.attachmentMaxBytes,
		})
	}
	return files
}

// checkWorkspaceLimit returns the state the files of a mail of size bytes take in the workspace tenant.
func (f mailFiles) checkWorkspaceLimit(ctx context.Context, tenant uuid.UUID, size int64) (string, error) {
	limit, err := readWorkspaceLimit(ctx, f.settings, f.logger, tenant, int64(f.caps.workspaceMaxBytes))
	if err != nil {
		return "", err
	}
	if limit == 0 {
		return fileStored, nil
	}
	var held int64
	err = f.pool.QueryRow(ctx, `SELECT (
			coalesce((SELECT used_bytes FROM plugin_email.workspace_usage WHERE tenant_id = $1), 0)
			+ coalesce((SELECT sum(size) FROM plugin_email.pending_files WHERE tenant_id = $1), 0)
		)::bigint`,
		tenant,
	).Scan(&held)
	if err != nil {
		return "", fmt.Errorf("email: read the bytes a workspace holds: %w", err)
	}
	if held+size > limit {
		return fileOverLimit, nil
	}
	return fileStored, nil
}

// keepFiles keeps each file of a mail of the workspace tenant whose files take state, queueing a failed run to delete.
func (f mailFiles) keepFiles(
	ctx context.Context, tenant uuid.UUID, files []mailFile, state string,
) ([]keptFile, error) {
	kept, err := prepareFiles(files, state)
	if err != nil {
		return nil, err
	}
	if err := insertWritingRows(ctx, f.pool, tenant, kept); err != nil {
		return nil, err
	}
	if err := f.saveFiles(ctx, tenant, files, kept); err != nil {
		return nil, errors.Join(err, f.queueFilesForDeletion(ctx, tenant, kept))
	}
	return kept, nil
}

// prepareFiles returns the first state of each file of a mail whose files take state, with an id for each one to send.
func prepareFiles(files []mailFile, state string) ([]keptFile, error) {
	kept := make([]keptFile, len(files))
	for i, file := range files {
		sum := sha256.Sum256(file.content)
		kept[i] = keptFile{
			state: state, contentType: storableType(file.contentType), size: int64(len(file.content)), sha256: sum[:],
		}
		switch {
		case file.tooLarge:
			kept[i].state = fileTooLarge
		case state == fileStored:
			id, err := uuid.NewV7()
			if err != nil {
				return nil, fmt.Errorf("email: make a file id: %w", err)
			}
			kept[i].queueID = id
		}
	}
	return kept, nil
}

// storableType returns contentType when it reads as a type and a subtype, application/octet-stream otherwise.
func storableType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.Contains(mediaType, "/") {
		return octetStream
	}
	return contentType
}

// saveFiles saves each file of kept that holds a queue row, marking one the store refuses as too large.
func (f mailFiles) saveFiles(ctx context.Context, tenant uuid.UUID, files []mailFile, kept []keptFile) error {
	ctx = sdk.WithTenant(ctx, tenant)
	for i := range kept {
		if kept[i].queueID == uuid.Nil {
			continue
		}
		saved, err := f.store.SaveAs(ctx, kept[i].queueID, kept[i].contentType, bytes.NewReader(files[i].content))
		switch {
		case errors.Is(err, sdk.ErrFileTooLarge):
			kept[i].state = fileTooLarge
		case err != nil:
			return fmt.Errorf("%w: %w", errFileStore, err)
		default:
			kept[i].contentType = saved.ContentType
		}
	}
	return nil
}

// queueFilesForDeletion marks every queued file of a run deleting, even after ctx ends, within the clean up timeout.
func (f mailFiles) queueFilesForDeletion(ctx context.Context, tenant uuid.UUID, files []keptFile) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), f.cleanupTimeout)
	defer cancel()
	return markDeleting(ctx, f.pool, tenant, files)
}

// queueColumns returns the id and the size of each file of a run that holds a queue row, as two columns.
func queueColumns(files []keptFile) ([]uuid.UUID, []int64) {
	ids := make([]uuid.UUID, 0, len(files))
	sizes := make([]int64, 0, len(files))
	for _, file := range files {
		if file.queueID != uuid.Nil {
			ids = append(ids, file.queueID)
			sizes = append(sizes, file.size)
		}
	}
	return ids, sizes
}

// insertWritingRows queues every file of a run that holds a queue id as writing, in the workspace tenant.
func insertWritingRows(ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID, files []keptFile) error {
	return writeQueueRows(ctx, pool, "queue the files", `INSERT INTO plugin_email.pending_files
			(tenant_id, file_id, state, size)
		SELECT $1, queued.file_id, 'writing', queued.size FROM unnest($2::uuid[], $3::bigint[]) AS queued (file_id, size)`,
		tenant, files)
}

// markDeleting writes each queued file of a run as deleting, restoring a row the sweep took.
func markDeleting(ctx context.Context, db statementRunner, tenant uuid.UUID, files []keptFile) error {
	return writeQueueRows(ctx, db, "queue the files for deletion", `INSERT INTO plugin_email.pending_files
			(tenant_id, file_id, state, size)
		SELECT $1, queued.file_id, 'deleting', queued.size FROM unnest($2::uuid[], $3::bigint[]) AS queued (file_id, size)
		ON CONFLICT (tenant_id, file_id) DO UPDATE SET state = 'deleting'`,
		tenant, files)
}

// writeQueueRows runs statement over the workspace tenant and the queued files of a run, its failure naming action.
func writeQueueRows(
	ctx context.Context, db statementRunner, action, statement string, tenant uuid.UUID, files []keptFile,
) error {
	ids, sizes := queueColumns(files)
	if _, err := db.Exec(ctx, statement, tenant, ids, sizes); err != nil {
		return fmt.Errorf("email: %s: %w", action, err)
	}
	return nil
}

// dequeueFiles removes the writing rows of a run's files inside tx, errFilesSwept when the sweep took any of them.
func dequeueFiles(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, files []keptFile) error {
	ids, _ := queueColumns(files)
	tag, err := tx.Exec(ctx, `DELETE FROM plugin_email.pending_files
		WHERE tenant_id = $1 AND file_id = ANY($2) AND state = 'writing'`,
		tenant, ids)
	if err != nil {
		return fmt.Errorf("email: dequeue the files: %w", err)
	}
	if tag.RowsAffected() != int64(len(ids)) {
		return errFilesSwept
	}
	return nil
}

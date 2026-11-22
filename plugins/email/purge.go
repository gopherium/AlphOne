// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

var (
	// errThreadNotFound reports a thread the workspace holds no row for.
	errThreadNotFound = errors.New("email: the workspace holds no such thread")
	// errMessageNotFound reports a mail the workspace, or the thread named with it, holds no row for.
	errMessageNotFound = errors.New("email: the workspace or the thread named holds no such mail")
	// errAttachmentNotFound reports a file the mail named with it holds no row for.
	errAttachmentNotFound = errors.New("email: the mail holds no such file")
	// errInsideSyncWindow reports a mail a mailbox sync brought in that is younger than the sync window.
	errInsideSyncWindow = errors.New("email: a mail a mailbox sync brought in is younger than the sync window")
	// errNothingToDelete reports a delete of files that names no attachment and not the original.
	errNothingToDelete = errors.New("email: the delete names no attachment and not the original")
)

var (
	// threadNotFound rejects a delete of a thread the workspace does not hold.
	threadNotFound = sdk.GraphError{Code: "NOT_FOUND", Reason: "thread_not_found", Err: errThreadNotFound}
	// messageNotFound rejects a delete of a mail the workspace, or the thread named with it, does not hold.
	messageNotFound = sdk.GraphError{Code: "NOT_FOUND", Reason: "message_not_found", Err: errMessageNotFound}
	// attachmentNotFound rejects a delete of a file the mail named with it does not hold.
	attachmentNotFound = sdk.GraphError{Code: "NOT_FOUND", Reason: "attachment_not_found", Err: errAttachmentNotFound}
	// nothingToDelete rejects a delete of files that names no attachment and not the original.
	nothingToDelete = sdk.GraphError{Code: "VALIDATION", Reason: "nothing_to_delete", Err: errNothingToDelete}
)

// mailToDelete is one mail of a thread a whole delete may take, beside whether a mailbox sync brought it in.
type mailToDelete struct {
	// id names the mail.
	id uuid.UUID
	// receivedAt is when the mailbox received the mail.
	receivedAt time.Time
	// synced reports a mail holding a reference that neither email:ingest nor the seed wrote.
	synced bool
}

// threadDelete names the mail of one thread a whole delete takes, and the time and the sync window it is judged by.
type threadDelete struct {
	// thread names the thread.
	thread uuid.UUID
	// message names the one mail of the thread to take, nil for every mail of it.
	message *uuid.UUID
	// at is when the delete runs, kept as the time each deleted key went.
	at time.Time
	// window is how long ago a mail a sync brought in must have been received for the delete to take it.
	window time.Duration
}

// threadDeletion is what one delete of a thread's mail took.
type threadDeletion struct {
	// messages names each mail the delete took, oldest first.
	messages []uuid.UUID
	// threadDeleted reports a thread the delete emptied and removed.
	threadDeleted bool
}

// filesDelete names the files of one mail a delete takes.
type filesDelete struct {
	// message names the mail.
	message uuid.UUID
	// attachment names the one attachment of the mail to take, nil for none.
	attachment *uuid.UUID
	// original asks for the original of the mail.
	original bool
}

// filesDeletion is what one delete of a mail's files took.
type filesDeletion struct {
	// attachment reports an attachment the delete took.
	attachment bool
	// original reports an original the delete took.
	original bool
}

// freedFile is what deleting one file of a mail took, nothing for a file that was not kept.
type freedFile struct {
	// deleted reports a kept file the delete took.
	deleted bool
	// size is the size of the file the delete took.
	size int64
}

// removedMail is what deleting the rows of some mail took.
type removedMail struct {
	// count counts the mails deleted.
	count int
	// storedBytes sums what the mails deleted added to their workspace.
	storedBytes int64
	// threads names each thread a deleted mail sat in.
	threads []uuid.UUID
}

// threadLeft is a thread that keeps mail after a delete, beside the subject and the times of the mail it keeps.
type threadLeft struct {
	// id names the thread.
	id uuid.UUID
	// subject is the subject of the earliest mail the thread keeps.
	subject string
	// firstMessageAt is when the mailbox received the earliest mail the thread keeps.
	firstMessageAt time.Time
	// lastMessageAt is when the mailbox received the newest mail the thread keeps.
	lastMessageAt time.Time
}

// deleteMessages deletes the mail of ids in the workspace tenant inside tx under the mailbox lock and counts it.
func deleteMessages(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ids []uuid.UUID, deletedAt time.Time,
) (int, error) {
	if err := guardDeletedKeys(ctx, tx, tenant, ids, deletedAt); err != nil {
		return 0, err
	}
	if err := queueMailFiles(ctx, tx, tenant, ids); err != nil {
		return 0, err
	}
	removed, err := deleteMailRows(ctx, tx, tenant, ids)
	if err != nil {
		return 0, err
	}
	if err := fixThreadsLeft(ctx, tx, tenant, removed.threads); err != nil {
		return 0, err
	}
	if err := takeStoredBytes(ctx, tx, tenant, removed.storedBytes); err != nil {
		return 0, err
	}
	return removed.count, nil
}

// guardDeletedKeys keeps the dedupe key of each mail of ids that holds one, beside the mail's id and deletedAt.
func guardDeletedKeys(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ids []uuid.UUID, deletedAt time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO plugin_email.deleted_dedupe_keys
			(tenant_id, mailbox_id, dedupe_key, deleted_message_id, deleted_at)
		SELECT $1, m.mailbox_id, m.dedupe_key, m.id, $3 FROM plugin_email.messages m
		WHERE m.tenant_id = $1 AND m.id = ANY($2) AND m.dedupe_key IS NOT NULL`,
		tenant, ids, deletedAt)
	if err != nil {
		return fmt.Errorf("email: guard the keys of deleted mail: %w", err)
	}
	return nil
}

// queueMailFiles queues the kept original and each kept attachment of the mails of ids for deletion with its size.
func queueMailFiles(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ids []uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO plugin_email.pending_files (tenant_id, file_id, state, size)
		SELECT $1, kept.file_id, 'deleting', kept.size FROM (
			SELECT m.raw_file_id AS file_id, m.raw_size AS size FROM plugin_email.messages m
			WHERE m.tenant_id = $1 AND m.id = ANY($2) AND m.raw_file_id IS NOT NULL
			UNION ALL
			SELECT a.file_id, a.size FROM plugin_email.attachments a
			WHERE a.tenant_id = $1 AND a.message_id = ANY($2) AND a.file_id IS NOT NULL
		) kept
		ON CONFLICT (tenant_id, file_id) DO UPDATE SET state = 'deleting'`,
		tenant, ids)
	if err != nil {
		return fmt.Errorf("email: queue the files of deleted mail: %w", err)
	}
	return nil
}

// deleteMailRows deletes the mails of ids with their attachment rows, answering what the mails held.
func deleteMailRows(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ids []uuid.UUID) (removedMail, error) {
	var removed removedMail
	err := tx.QueryRow(ctx, `WITH files AS (
			DELETE FROM plugin_email.attachments a WHERE a.tenant_id = $1 AND a.message_id = ANY($2)
		), gone AS (
			DELETE FROM plugin_email.messages m WHERE m.tenant_id = $1 AND m.id = ANY($2)
			RETURNING m.thread_id, m.stored_bytes
		)
		SELECT count(*), coalesce(sum(gone.stored_bytes), 0)::bigint,
			coalesce(array_agg(DISTINCT gone.thread_id), '{}')
		FROM gone`,
		tenant, ids,
	).Scan(&removed.count, &removed.storedBytes, &removed.threads)
	if err != nil {
		return removedMail{}, fmt.Errorf("email: delete the rows of mail: %w", err)
	}
	return removed, nil
}

// fixThreadsLeft removes each of threads left with no mail and fits each one that keeps mail to the mail it keeps.
func fixThreadsLeft(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, threads []uuid.UUID) error {
	rows, _ := tx.Query(ctx, `WITH emptied AS (
			DELETE FROM plugin_email.threads t
			WHERE t.tenant_id = $1 AND t.id = ANY($2) AND NOT EXISTS (
				SELECT FROM plugin_email.messages m WHERE m.tenant_id = $1 AND m.thread_id = t.id)
		)
		SELECT t.id, earliest.subject, earliest.received_at,
			(SELECT max(l.received_at) FROM plugin_email.messages l WHERE l.tenant_id = $1 AND l.thread_id = t.id)
		FROM plugin_email.threads t
		JOIN LATERAL (SELECT m.subject, m.received_at FROM plugin_email.messages m
			WHERE m.tenant_id = $1 AND m.thread_id = t.id
			ORDER BY m.received_at, m.id
			LIMIT 1) earliest ON TRUE
		WHERE t.tenant_id = $1 AND t.id = ANY($2)
		ORDER BY t.id`,
		tenant, threads)
	left, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (threadLeft, error) {
		var thread threadLeft
		err := row.Scan(&thread.id, &thread.subject, &thread.firstMessageAt, &thread.lastMessageAt)
		return thread, err
	})
	if err != nil {
		return fmt.Errorf("email: read the threads left: %w", err)
	}
	for _, thread := range left {
		if err := fixThreadLeft(ctx, tx, tenant, thread); err != nil {
			return err
		}
	}
	return nil
}

// fixThreadLeft gives a thread that keeps mail the subject, the subject key and the times of the mail it keeps.
func fixThreadLeft(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, thread threadLeft) error {
	_, err := tx.Exec(ctx, `UPDATE plugin_email.threads SET subject = $3, subject_key = $4,
			first_message_at = $5, last_message_at = $6
		WHERE tenant_id = $1 AND id = $2`,
		tenant, thread.id, thread.subject, subjectKeyOf(thread.subject), thread.firstMessageAt, thread.lastMessageAt)
	if err != nil {
		return fmt.Errorf("email: fit a thread to the mail left: %w", err)
	}
	return nil
}

// takeStoredBytes takes size off the bytes the workspace tenant keeps inside tx, never below zero.
func takeStoredBytes(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, size int64) error {
	_, err := tx.Exec(ctx, `UPDATE plugin_email.workspace_usage SET used_bytes = GREATEST(used_bytes - $2, 0)
		WHERE tenant_id = $1`,
		tenant, size)
	if err != nil {
		return fmt.Errorf("email: take the bytes of deleted mail: %w", err)
	}
	return nil
}

// inMailboxTx runs step inside one transaction over the mailbox of the workspace tenant, missing for none.
func inMailboxTx[T any](
	ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID, missing error,
	step func(tx pgx.Tx, mailboxID uuid.UUID) (T, error),
) (T, error) {
	var answer T
	box, held, err := readWorkspaceMailbox(ctx, pool, tenant)
	if err != nil {
		return answer, err
	}
	if !held {
		return answer, missing
	}
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var err error
		answer, err = step(tx, box.id)
		return err
	})
	return answer, err
}

// deleteThreadMail deletes one mail of a thread of the workspace tenant, or every mail of it, in one transaction.
func deleteThreadMail(
	ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID, request threadDelete,
) (threadDeletion, error) {
	return inMailboxTx(ctx, pool, tenant, threadNotFound, func(tx pgx.Tx, mailboxID uuid.UUID) (threadDeletion, error) {
		return deleteLockedThreadMail(ctx, tx, tenant, mailboxID, request)
	})
}

// deleteLockedThreadMail deletes the mail of the thread request names inside tx once it holds the mailbox lock.
func deleteLockedThreadMail(
	ctx context.Context, tx pgx.Tx, tenant, mailboxID uuid.UUID, request threadDelete,
) (threadDeletion, error) {
	if err := lockMailbox(ctx, tx, tenant, mailboxID); err != nil {
		return threadDeletion{}, err
	}
	thread, err := readMailToDelete(ctx, tx, tenant, request.thread)
	if err != nil {
		return threadDeletion{}, err
	}
	chosen, err := pickMailToDelete(thread, request.message)
	if err != nil {
		return threadDeletion{}, err
	}
	if err := checkSyncWindow(chosen, request.at, request.window); err != nil {
		return threadDeletion{}, err
	}
	ids := mapEach(chosen, func(mail mailToDelete) uuid.UUID { return mail.id })
	if _, err := deleteMessages(ctx, tx, tenant, ids, request.at); err != nil {
		return threadDeletion{}, err
	}
	return threadDeletion{messages: ids, threadDeleted: len(chosen) == len(thread)}, nil
}

// readMailToDelete returns every mail of one thread of the workspace tenant oldest first, thread_not_found for none.
func readMailToDelete(ctx context.Context, tx pgx.Tx, tenant, thread uuid.UUID) ([]mailToDelete, error) {
	rows, _ := tx.Query(ctx, `SELECT m.id, m.received_at, EXISTS (SELECT FROM plugin_email.provider_refs r
			WHERE r.tenant_id = $1 AND r.message_id = m.id
				AND NOT starts_with(r.provider_ref, $3) AND NOT starts_with(r.provider_ref, $4))
		FROM plugin_email.messages m
		WHERE m.tenant_id = $1 AND m.thread_id = $2
		ORDER BY m.received_at, m.id`,
		tenant, thread, filePrefix, seedPrefix)
	mails, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (mailToDelete, error) {
		var mail mailToDelete
		err := row.Scan(&mail.id, &mail.receivedAt, &mail.synced)
		return mail, err
	})
	if err != nil {
		return nil, fmt.Errorf("email: read the mail to delete: %w", err)
	}
	if len(mails) == 0 {
		return nil, threadNotFound
	}
	return mails, nil
}

// pickMailToDelete returns the mail of thread message names, every mail of thread for nil, message_not_found for none.
func pickMailToDelete(thread []mailToDelete, message *uuid.UUID) ([]mailToDelete, error) {
	if message == nil {
		return thread, nil
	}
	at := slices.IndexFunc(thread, func(mail mailToDelete) bool { return mail.id == *message })
	if at < 0 {
		return nil, messageNotFound
	}
	return thread[at : at+1], nil
}

// checkSyncWindow rejects the delete of mails when one a sync brought in was received at or after at less window.
func checkSyncWindow(mails []mailToDelete, at time.Time, window time.Duration) error {
	start := at.Add(-window)
	young := slices.ContainsFunc(mails, func(mail mailToDelete) bool {
		return mail.synced && !mail.receivedAt.Before(start)
	})
	if young {
		return sdk.GraphError{
			Code: "CONFLICT", Reason: "inside_sync_window", Meta: map[string]any{"window": window.String()},
			Err: errInsideSyncWindow,
		}
	}
	return nil
}

// deleteFiles deletes the attachment, the original or both of one mail of the workspace tenant in one transaction.
func deleteFiles(
	ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID, request filesDelete,
) (filesDeletion, error) {
	if request.attachment == nil && !request.original {
		return filesDeletion{}, nothingToDelete
	}
	return inMailboxTx(ctx, pool, tenant, messageNotFound, func(tx pgx.Tx, mailboxID uuid.UUID) (filesDeletion, error) {
		return deleteLockedFiles(ctx, tx, tenant, mailboxID, request)
	})
}

// deleteLockedFiles deletes the files of the mail request names inside tx once it holds the mailbox lock.
func deleteLockedFiles(
	ctx context.Context, tx pgx.Tx, tenant, mailboxID uuid.UUID, request filesDelete,
) (filesDeletion, error) {
	if err := lockMailbox(ctx, tx, tenant, mailboxID); err != nil {
		return filesDeletion{}, err
	}
	if err := lockMailToFree(ctx, tx, tenant, request.message); err != nil {
		return filesDeletion{}, err
	}
	attachment, err := deleteAttachmentFile(ctx, tx, tenant, request.message, request.attachment)
	if err != nil {
		return filesDeletion{}, err
	}
	original, err := deleteOriginalFile(ctx, tx, tenant, request.message, request.original)
	if err != nil {
		return filesDeletion{}, err
	}
	if err := takeFreedBytes(ctx, tx, tenant, request.message, attachment.size+original.size); err != nil {
		return filesDeletion{}, err
	}
	return filesDeletion{attachment: attachment.deleted, original: original.deleted}, nil
}

// lockMailToFree holds the row of the mail id names in the workspace tenant until tx ends, message_not_found for none.
func lockMailToFree(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	var locked uuid.UUID
	err := tx.QueryRow(ctx, "SELECT id FROM plugin_email.messages WHERE tenant_id = $1 AND id = $2 FOR UPDATE",
		tenant, id,
	).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return messageNotFound
	}
	if err != nil {
		return fmt.Errorf("email: lock the mail to free: %w", err)
	}
	return nil
}

// deleteAttachmentFile deletes the kept file of the attachment id names of the mail messageID, nothing for nil.
func deleteAttachmentFile(
	ctx context.Context, tx pgx.Tx, tenant, messageID uuid.UUID, id *uuid.UUID,
) (freedFile, error) {
	if id == nil {
		return freedFile{}, nil
	}
	var found bool
	var freed freedFile
	err := tx.QueryRow(ctx, `WITH file AS (
			SELECT a.id, a.file_id, a.size, a.state = 'stored' AND a.file_id IS NOT NULL AS kept
			FROM plugin_email.attachments a
			WHERE a.tenant_id = $1 AND a.message_id = $2 AND a.id = $3
		), queued AS (
			INSERT INTO plugin_email.pending_files (tenant_id, file_id, state, size)
			SELECT $1, file.file_id, 'deleting', file.size FROM file WHERE file.kept
			ON CONFLICT (tenant_id, file_id) DO UPDATE SET state = 'deleting'
		), freed AS (
			UPDATE plugin_email.attachments a SET state = 'deleted', file_id = NULL
			FROM file WHERE a.tenant_id = $1 AND a.id = file.id AND file.kept
			RETURNING file.size
		)
		SELECT EXISTS (SELECT FROM file), EXISTS (SELECT FROM freed),
			coalesce((SELECT sum(freed.size) FROM freed), 0)::bigint`,
		tenant, messageID, *id,
	).Scan(&found, &freed.deleted, &freed.size)
	if err != nil {
		return freedFile{}, fmt.Errorf("email: delete the file of an attachment: %w", err)
	}
	if !found {
		return freedFile{}, attachmentNotFound
	}
	return freed, nil
}

// deleteOriginalFile deletes the kept original of the mail id names when wanted, nothing when it is not.
func deleteOriginalFile(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, wanted bool) (freedFile, error) {
	if !wanted {
		return freedFile{}, nil
	}
	var freed freedFile
	err := tx.QueryRow(ctx, `WITH mail AS (
			SELECT m.id, m.raw_file_id, m.raw_size FROM plugin_email.messages m
			WHERE m.tenant_id = $1 AND m.id = $2 AND m.raw_state = 'stored' AND m.raw_file_id IS NOT NULL
		), queued AS (
			INSERT INTO plugin_email.pending_files (tenant_id, file_id, state, size)
			SELECT $1, mail.raw_file_id, 'deleting', mail.raw_size FROM mail
			ON CONFLICT (tenant_id, file_id) DO UPDATE SET state = 'deleting'
		), freed AS (
			UPDATE plugin_email.messages m SET raw_state = 'deleted', raw_file_id = NULL
			FROM mail WHERE m.tenant_id = $1 AND m.id = mail.id
			RETURNING mail.raw_size
		)
		SELECT EXISTS (SELECT FROM freed), coalesce((SELECT sum(freed.raw_size) FROM freed), 0)::bigint`,
		tenant, id,
	).Scan(&freed.deleted, &freed.size)
	if err != nil {
		return freedFile{}, fmt.Errorf("email: delete the original of a mail: %w", err)
	}
	return freed, nil
}

// takeFreedBytes takes size off the bytes of the mail id names and of its workspace tenant, never below zero.
func takeFreedBytes(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, size int64) error {
	_, err := tx.Exec(ctx, `WITH mail AS (
			UPDATE plugin_email.messages SET stored_bytes = GREATEST(stored_bytes - $3, 0)
			WHERE tenant_id = $1 AND id = $2 AND $3 > 0
		)
		UPDATE plugin_email.workspace_usage SET used_bytes = GREATEST(used_bytes - $3, 0)
		WHERE tenant_id = $1 AND $3 > 0`,
		tenant, id, size)
	if err != nil {
		return fmt.Errorf("email: take the bytes of freed files: %w", err)
	}
	return nil
}

// wasDeleted reports whether the mailbox of the workspace tenant deleted a mail of the dedupe key.
func wasDeleted(ctx context.Context, db rowQuerier, tenant, mailboxID uuid.UUID, key []byte) (bool, error) {
	var deleted bool
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT FROM plugin_email.deleted_dedupe_keys
		WHERE tenant_id = $1 AND mailbox_id = $2 AND dedupe_key = $3)`,
		tenant, mailboxID, key,
	).Scan(&deleted)
	if err != nil {
		return false, fmt.Errorf("email: read the keys of deleted mail: %w", err)
	}
	return deleted, nil
}

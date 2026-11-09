// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	// providerThreadKind is the kind of a key that hashes the provider's own thread id.
	providerThreadKind = "provider_thread"
	// internetMessageIDKind is the kind of a key that hashes a Message-ID.
	internetMessageIDKind = "internet_message_id"
	// threadIndexKind is the kind of a key that hashes the conversation GUID of a Thread-Index.
	threadIndexKind = "thread_index"
)

// threadKey is one id that leads to a thread, its kind beside the SHA-256 of the id.
type threadKey struct {
	// kind says which id the key hashes.
	kind string
	// hash is the SHA-256 of the id.
	hash [32]byte
}

// threadSpan is one thread beside the times of its first and last mail.
type threadSpan struct {
	// id names the thread.
	id uuid.UUID
	// firstMessageAt is when the first mail of the thread was received.
	firstMessageAt time.Time
	// lastMessageAt is when the last mail of the thread was received.
	lastMessageAt time.Time
}

// keyedThread is the thread a key leads to, beside whether a stored mail holds the key.
type keyedThread struct {
	// thread is the thread the key points at.
	thread threadSpan
	// mailHeld reports a key a stored mail holds, false for a placeholder some mail only mentions.
	mailHeld bool
}

// threadEntry is what a thread takes from one of its mails.
type threadEntry struct {
	// subject is the subject the mail keeps.
	subject string
	// subjectKey is the SHA-256 of the lowercased normalized subject, nil when that is empty.
	subjectKey []byte
	// at is when the mailbox received the mail.
	at time.Time
}

// subjectSearch is what the subject step looks for in one mailbox.
type subjectSearch struct {
	// mailboxID names the mailbox searched.
	mailboxID uuid.UUID
	// subjectKey is the subject key a thread must hold.
	subjectKey []byte
	// at is when the mailbox received the mail looking for a thread.
	at time.Time
	// window is how far from its first or last mail a thread still takes the mail.
	window time.Duration
	// senders are the contacts a thread must hold one of.
	senders []uuid.UUID
}

// keyColumns returns the kinds and the hashes of keys, in order, as two columns.
func keyColumns(keys []threadKey) ([]string, [][]byte) {
	kinds, hashes := make([]string, len(keys)), make([][]byte, len(keys))
	for i, key := range keys {
		kinds[i], hashes[i] = key.kind, key.hash[:]
	}
	return kinds, hashes
}

// insertThread starts a thread of the mailbox in the workspace tenant at its first mail and returns its new id.
func insertThread(ctx context.Context, tx pgx.Tx, tenant, mailboxID uuid.UUID, entry threadEntry) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("email: make a thread id: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO plugin_email.threads
			(id, tenant_id, mailbox_id, subject, subject_key, first_message_at, last_message_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		id, tenant, mailboxID, entry.subject, entry.subjectKey, entry.at)
	if err != nil {
		return uuid.Nil, fmt.Errorf("email: insert thread: %w", err)
	}
	return id, nil
}

// claimThreadKeys points each free key of the mailbox at a thread as a placeholder, leaving every held key where it is.
func claimThreadKeys(ctx context.Context, tx pgx.Tx, tenant, mailboxID, threadID uuid.UUID, keys []threadKey) error {
	kinds, hashes := keyColumns(keys)
	_, err := tx.Exec(ctx, `INSERT INTO plugin_email.thread_keys (tenant_id, mailbox_id, kind, key, thread_id)
		SELECT $1, $2, listed.kind, listed.key, $3 FROM unnest($4::text[], $5::bytea[]) AS listed (kind, key)
		ON CONFLICT (tenant_id, mailbox_id, kind, key) DO NOTHING`,
		tenant, mailboxID, threadID, kinds, hashes)
	if err != nil {
		return fmt.Errorf("email: claim thread keys: %w", err)
	}
	return nil
}

// holdMessageIDKey points the key of a stored mail's own Message-ID at its thread and the mail, unless a mail holds it.
func holdMessageIDKey(
	ctx context.Context, tx pgx.Tx, tenant, mailboxID, threadID, messageID uuid.UUID, hash [32]byte,
) error {
	_, err := tx.Exec(ctx, `INSERT INTO plugin_email.thread_keys
			(tenant_id, mailbox_id, kind, key, thread_id, message_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, mailbox_id, kind, key) DO UPDATE
		SET thread_id = EXCLUDED.thread_id, message_id = EXCLUDED.message_id
		WHERE plugin_email.thread_keys.message_id IS NULL`,
		tenant, mailboxID, internetMessageIDKind, hash[:], threadID, messageID)
	if err != nil {
		return fmt.Errorf("email: hold message id key: %w", err)
	}
	return nil
}

// readKeyedThreads returns the thread each of keys leads to in the mailbox, leaving out the keys it holds no row for.
func readKeyedThreads(
	ctx context.Context, tx pgx.Tx, tenant, mailboxID uuid.UUID, keys []threadKey,
) (map[threadKey]keyedThread, error) {
	kinds, hashes := keyColumns(keys)
	rows, _ := tx.Query(ctx, `SELECT k.kind, k.key, k.message_id IS NOT NULL, t.id, t.first_message_at,
			t.last_message_at
		FROM plugin_email.thread_keys k
		JOIN plugin_email.threads t ON t.id = k.thread_id
		WHERE k.tenant_id = $1 AND k.mailbox_id = $2 AND (k.kind, k.key) IN (
			SELECT listed.kind, listed.key FROM unnest($3::text[], $4::bytea[]) AS listed (kind, key))`,
		tenant, mailboxID, kinds, hashes)
	keyed := map[threadKey]keyedThread{}
	var key threadKey
	var hash []byte
	var thread keyedThread
	_, err := pgx.ForEachRow(rows, []any{
		&key.kind, &hash, &thread.mailHeld, &thread.thread.id, &thread.thread.firstMessageAt,
		&thread.thread.lastMessageAt,
	}, func() error {
		copy(key.hash[:], hash)
		keyed[key] = thread
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("email: read keyed threads: %w", err)
	}
	return keyed, nil
}

// readThreadContacts returns the contacts linked to a mail of each thread, leaving out a thread that holds none.
func readThreadContacts(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, threads []uuid.UUID,
) (map[uuid.UUID][]uuid.UUID, error) {
	rows, _ := tx.Query(ctx, `SELECT DISTINCT m.thread_id, c.contact_id
		FROM plugin_email.messages m
		JOIN plugin_email.message_contacts c ON c.message_id = m.id
		WHERE m.tenant_id = $1 AND m.thread_id = ANY($2)
		ORDER BY m.thread_id, c.contact_id`,
		tenant, threads)
	contacts := map[uuid.UUID][]uuid.UUID{}
	var thread, contact uuid.UUID
	_, err := pgx.ForEachRow(rows, []any{&thread, &contact}, func() error {
		contacts[thread] = append(contacts[thread], contact)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("email: read thread contacts: %w", err)
	}
	return contacts, nil
}

// readSubjectThread returns the thread with the newest mail that the search finds, and whether it finds one.
func readSubjectThread(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, search subjectSearch,
) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT t.id FROM plugin_email.threads t
		WHERE t.tenant_id = $1 AND t.mailbox_id = $2 AND t.subject_key = $3
			AND (t.first_message_at BETWEEN $4 AND $5 OR t.last_message_at BETWEEN $4 AND $5)
			AND EXISTS (SELECT FROM plugin_email.messages m
				JOIN plugin_email.message_contacts c ON c.message_id = m.id
				WHERE m.tenant_id = $1 AND m.thread_id = t.id AND c.contact_id = ANY($6))
		ORDER BY t.last_message_at DESC, t.id DESC
		LIMIT 1`,
		tenant, search.mailboxID, search.subjectKey, search.at.Add(-search.window), search.at.Add(search.window),
		search.senders,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("email: read subject thread: %w", err)
	}
	return id, true, nil
}

// mergeThreads moves the mails and keys of the newer of two threads to the older one, removes it and returns the older.
func mergeThreads(ctx context.Context, tx pgx.Tx, tenant, first, second uuid.UUID) (uuid.UUID, error) {
	var kept uuid.UUID
	err := tx.QueryRow(ctx, `WITH pair AS (
			SELECT id, last_message_at, row_number() OVER (ORDER BY first_message_at, id) AS place
			FROM plugin_email.threads
			WHERE tenant_id = $1 AND id IN ($2, $3)
		), kept AS (
			SELECT id FROM pair WHERE place = 1
		), gone AS (
			SELECT id, last_message_at FROM pair WHERE place = 2
		), moved_messages AS (
			UPDATE plugin_email.messages m SET thread_id = kept.id FROM kept, gone
			WHERE m.tenant_id = $1 AND m.thread_id = gone.id
		), moved_keys AS (
			UPDATE plugin_email.thread_keys k SET thread_id = kept.id FROM kept, gone
			WHERE k.tenant_id = $1 AND k.thread_id = gone.id
		), removed AS (
			DELETE FROM plugin_email.threads t USING gone WHERE t.tenant_id = $1 AND t.id = gone.id
		)
		UPDATE plugin_email.threads t SET last_message_at = GREATEST(t.last_message_at, gone.last_message_at)
		FROM kept, gone
		WHERE t.tenant_id = $1 AND t.id = kept.id
		RETURNING t.id`,
		tenant, first, second,
	).Scan(&kept)
	if err != nil {
		return uuid.Nil, fmt.Errorf("email: merge threads: %w", err)
	}
	return kept, nil
}

// updateThreadSpan widens the times of a thread to take a mail, taking the mail's subject when it is the earliest.
func updateThreadSpan(ctx context.Context, tx pgx.Tx, tenant, threadID uuid.UUID, entry threadEntry) error {
	_, err := tx.Exec(ctx, `UPDATE plugin_email.threads SET
			subject = CASE WHEN $3 < first_message_at THEN $4 ELSE subject END,
			subject_key = CASE WHEN $3 < first_message_at THEN $5::bytea ELSE subject_key END,
			first_message_at = LEAST(first_message_at, $3),
			last_message_at = GREATEST(last_message_at, $3)
		WHERE tenant_id = $1 AND id = $2`,
		tenant, threadID, entry.at, entry.subject, entry.subjectKey)
	if err != nil {
		return fmt.Errorf("email: update thread span: %w", err)
	}
	return nil
}

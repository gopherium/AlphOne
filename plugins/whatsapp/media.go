// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mediaDescriptor identifies a WhatsApp media asset announced by a webhook message.
type mediaDescriptor struct {
	mediaID  string
	mimeType string
	sha256   string
	filename string
	voice    bool
	animated bool
}

// pgxExecutor runs SQL statements on a pool or inside a transaction.
type pgxExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// pendingMediaRow carries a claimed media download job.
type pendingMediaRow struct {
	MessageID      uuid.UUID `db:"message_id"`
	ConversationID uuid.UUID `db:"conversation_id"`
	MediaID        string    `db:"media_id"`
	SHA256         string    `db:"sha256"`
	Attempts       int       `db:"attempts"`
	CreatedAt      time.Time `db:"created_at"`
}

// insertMediaPending records a message's media asset as awaiting download.
func insertMediaPending(ctx context.Context, exec pgxExecutor, messageID uuid.UUID, d mediaDescriptor) error {
	now := time.Now().UTC()
	_, err := exec.Exec(ctx, `
		INSERT INTO plugin_whatsapp.media (message_id, media_id, status, mime_type, sha256, filename,
			voice, animated, next_attempt_at, created_at)
		VALUES ($1, $2, 'pending', $3, $4, NULLIF($5, ''), $6, $7, $8, $8)`,
		messageID, d.mediaID, d.mimeType, d.sha256, d.filename, d.voice, d.animated, now,
	)
	if err != nil {
		return fmt.Errorf("whatsapp: insert pending media: %w", err)
	}
	return nil
}

// claimDueMedia returns up to limit pending media rows due for a download attempt, oldest first.
func (s *store) claimDueMedia(ctx context.Context, now time.Time, limit int) ([]pendingMediaRow, error) {
	rows, _ := s.pool.Query(ctx, `
		SELECT med.message_id, msg.conversation_id, med.media_id, med.sha256, med.attempts, med.created_at
		FROM plugin_whatsapp.media med
		JOIN plugin_whatsapp.messages msg ON msg.id = med.message_id
		WHERE med.status = 'pending' AND med.next_attempt_at <= $1
		ORDER BY med.created_at, med.message_id
		LIMIT $2`,
		now, limit,
	)
	pending, err := pgx.CollectRows(rows, pgx.RowToStructByName[pendingMediaRow])
	if err != nil {
		return nil, fmt.Errorf("whatsapp: claim due media: %w", err)
	}
	return pending, nil
}

// markMediaStored saves a downloaded blob on a still pending media row.
func (s *store) markMediaStored(
	ctx context.Context, messageID uuid.UUID, data []byte, mimeType string, fileSize int64,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE plugin_whatsapp.media
		SET status = 'stored', data = $2, mime_type = $3, file_size = $4, last_error = NULL, stored_at = $5
		WHERE message_id = $1 AND status = 'pending'`,
		messageID, data, mimeType, fileSize, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("whatsapp: mark media stored: %w", err)
	}
	return nil
}

// markMediaFailed retires a still pending media row with the failure reason.
func (s *store) markMediaFailed(ctx context.Context, messageID uuid.UUID, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE plugin_whatsapp.media
		SET status = 'failed', last_error = $2
		WHERE message_id = $1 AND status = 'pending'`,
		messageID, reason,
	)
	if err != nil {
		return fmt.Errorf("whatsapp: mark media failed: %w", err)
	}
	return nil
}

// rescheduleMedia records a failed download attempt and defers the next one.
func (s *store) rescheduleMedia(
	ctx context.Context, messageID uuid.UUID, reason string, nextAttemptAt time.Time,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE plugin_whatsapp.media
		SET attempts = attempts + 1, last_error = $2, next_attempt_at = $3
		WHERE message_id = $1 AND status = 'pending'`,
		messageID, reason, nextAttemptAt,
	)
	if err != nil {
		return fmt.Errorf("whatsapp: reschedule media: %w", err)
	}
	return nil
}

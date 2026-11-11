// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	// permanentDatabaseError marks a mail the database refuses for good, which is never tried again.
	permanentDatabaseError = "permanent_database_error"
	// temporaryDatabaseError marks a mail that failed on a database error a retry can pass.
	temporaryDatabaseError = "temporary_database_error"
	// fileStoreError marks a mail one of whose files the file store could not save.
	fileStoreError = "file_store_error"
)

// failedMail is one mail that could not be stored, named by its reference alone.
type failedMail struct {
	// reason says what failed, one of the three fixed reasons.
	reason string
	// firstFailedAt is when the mail first failed.
	firstFailedAt time.Time
	// lastFailedAt is when the mail last failed.
	lastFailedAt time.Time
	// attempts counts how many times the mail failed.
	attempts int
}

// isSetAside reports whether the mail is never tried again at now, after a permanent error or past the retry window.
func (f failedMail) isSetAside(now time.Time, window time.Duration) bool {
	return f.reason == permanentDatabaseError || now.Sub(f.firstFailedAt) > window
}

// failureReason returns the fixed reason err sets a mail aside for, never the text of err.
func failureReason(err error) string {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, errFileStore):
		return fileStoreError
	case errors.As(err, &pgErr) && (strings.HasPrefix(pgErr.Code, "22") || strings.HasPrefix(pgErr.Code, "23")):
		return permanentDatabaseError
	}
	return temporaryDatabaseError
}

// recordFailure sets aside the mail ref names in the mailbox of the workspace tenant for reason, failed at failedAt.
func recordFailure(
	ctx context.Context, db statementRunner, tenant, mailboxID uuid.UUID, ref, reason string, failedAt time.Time,
) error {
	_, err := db.Exec(ctx, `INSERT INTO plugin_email.failed_mail
			(tenant_id, mailbox_id, provider_ref, reason, first_failed_at, last_failed_at, attempts)
		VALUES ($1, $2, $3, $4, $5, $5, 1)
		ON CONFLICT (tenant_id, mailbox_id, provider_ref) DO UPDATE
		SET reason = EXCLUDED.reason, last_failed_at = EXCLUDED.last_failed_at,
			attempts = plugin_email.failed_mail.attempts + 1`,
		tenant, mailboxID, ref, reason, failedAt)
	if err != nil {
		return fmt.Errorf("email: set the mail aside: %w", err)
	}
	return nil
}

// readFailedMail returns the failure of the mail ref names in the workspace tenant's mailbox, and whether it failed.
func readFailedMail(
	ctx context.Context, db rowQuerier, tenant, mailboxID uuid.UUID, ref string,
) (failedMail, bool, error) {
	var failure failedMail
	err := db.QueryRow(ctx, `SELECT reason, first_failed_at, last_failed_at, attempts FROM plugin_email.failed_mail
		WHERE tenant_id = $1 AND mailbox_id = $2 AND provider_ref = $3`,
		tenant, mailboxID, ref,
	).Scan(&failure.reason, &failure.firstFailedAt, &failure.lastFailedAt, &failure.attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return failedMail{}, false, nil
	}
	if err != nil {
		return failedMail{}, false, fmt.Errorf("email: read the failure of a mail: %w", err)
	}
	failure.firstFailedAt, failure.lastFailedAt = failure.firstFailedAt.UTC(), failure.lastFailedAt.UTC()
	return failure, true, nil
}

// deleteFailedMail removes the failure of the mail ref names in the mailbox of the workspace tenant.
func deleteFailedMail(ctx context.Context, db statementRunner, tenant, mailboxID uuid.UUID, ref string) error {
	_, err := db.Exec(ctx, `DELETE FROM plugin_email.failed_mail
		WHERE tenant_id = $1 AND mailbox_id = $2 AND provider_ref = $3`,
		tenant, mailboxID, ref)
	if err != nil {
		return fmt.Errorf("email: delete the failure of a mail: %w", err)
	}
	return nil
}

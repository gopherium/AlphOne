// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// workspaceUsage is the space the mail of one workspace takes, beside its limit.
type workspaceUsage struct {
	// usedBytes counts the bytes the workspace keeps and every file still queued, as the limit check counts them.
	usedBytes int64
	// freeingBytes counts the bytes of the files a delete queued that the sweep has not removed yet.
	freeingBytes int64
	// limitBytes is the most bytes the workspace may keep, zero for no limit.
	limitBytes int64
}

// readUsage returns the space the mail of the workspace tenant takes, beside its limit.
func (f mailFiles) readUsage(ctx context.Context, tenant uuid.UUID) (workspaceUsage, error) {
	limit, err := readWorkspaceLimit(ctx, f.settings, f.logger, tenant, int64(f.caps.workspaceMaxBytes))
	if err != nil {
		return workspaceUsage{}, err
	}
	usage := workspaceUsage{limitBytes: limit}
	err = f.pool.QueryRow(ctx, `SELECT (
			coalesce((SELECT used_bytes FROM plugin_email.workspace_usage WHERE tenant_id = $1), 0)
			+ coalesce(sum(p.size), 0)
		)::bigint, coalesce(sum(p.size) FILTER (WHERE p.state = 'deleting'), 0)::bigint
		FROM plugin_email.pending_files p WHERE p.tenant_id = $1`,
		tenant,
	).Scan(&usage.usedBytes, &usage.freeingBytes)
	if err != nil {
		return workspaceUsage{}, fmt.Errorf("email: read the space a workspace uses: %w", err)
	}
	return usage, nil
}

// readCostliestMails returns up to limit mails of the workspace tenant by the bytes each adds to it, ties by id.
func readCostliestMails(ctx context.Context, db querier, tenant uuid.UUID, limit int) ([]mailView, error) {
	rows, _ := db.Query(ctx, `SELECT `+mailColumns+` FROM plugin_email.messages m
		WHERE m.tenant_id = $1
		ORDER BY m.stored_bytes DESC, m.id DESC
		LIMIT $2`,
		tenant, limit)
	mails, err := pgx.CollectRows(rows, scanMail)
	if err != nil {
		return nil, fmt.Errorf("email: read the mail that costs the most: %w", err)
	}
	return mails, nil
}

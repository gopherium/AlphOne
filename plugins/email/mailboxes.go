// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mailbox is one connected mailbox of a workspace.
type mailbox struct {
	// id names the mailbox.
	id uuid.UUID
	// address is the mailbox's own address.
	address string
	// aliases are the other addresses that reach the mailbox.
	aliases []string
}

// ownAddresses returns the mailbox's address followed by every alias.
func (m mailbox) ownAddresses() []string {
	return append([]string{m.address}, m.aliases...)
}

// ensureMailbox returns the mailbox of the workspace tenant, storing one at address with aliases when it holds none.
func ensureMailbox(
	ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID, address string, aliases []string,
) (mailbox, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return mailbox{}, fmt.Errorf("email: make a mailbox id: %w", err)
	}
	var box mailbox
	err = pool.QueryRow(ctx, `INSERT INTO plugin_email.mailboxes (id, tenant_id, address, aliases)
		VALUES ($1, $2, $3, COALESCE($4::text[], '{}'))
		ON CONFLICT (tenant_id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id
		RETURNING id, address, aliases`,
		id, tenant, address, aliases,
	).Scan(&box.id, &box.address, &box.aliases)
	if err != nil {
		return mailbox{}, fmt.Errorf("email: ensure mailbox: %w", err)
	}
	return box, nil
}

// readMailbox returns the mailbox id names in the workspace tenant, an error wrapping pgx.ErrNoRows when none.
func readMailbox(ctx context.Context, pool *pgxpool.Pool, tenant, id uuid.UUID) (mailbox, error) {
	var box mailbox
	err := pool.QueryRow(ctx,
		"SELECT id, address, aliases FROM plugin_email.mailboxes WHERE tenant_id = $1 AND id = $2",
		tenant, id,
	).Scan(&box.id, &box.address, &box.aliases)
	if err != nil {
		return mailbox{}, fmt.Errorf("email: read mailbox: %w", err)
	}
	return box, nil
}

// readWorkspaceMailbox returns the mailbox of the workspace tenant, and whether the workspace holds one.
func readWorkspaceMailbox(ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID) (mailbox, bool, error) {
	var box mailbox
	err := pool.QueryRow(ctx,
		"SELECT id, address, aliases FROM plugin_email.mailboxes WHERE tenant_id = $1",
		tenant,
	).Scan(&box.id, &box.address, &box.aliases)
	if errors.Is(err, pgx.ErrNoRows) {
		return mailbox{}, false, nil
	}
	if err != nil {
		return mailbox{}, false, fmt.Errorf("email: read the workspace mailbox: %w", err)
	}
	return box, true, nil
}

// lockMailbox holds the row of the mailbox id names in the workspace tenant until tx ends.
func lockMailbox(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	var locked uuid.UUID
	err := tx.QueryRow(ctx,
		"SELECT id FROM plugin_email.mailboxes WHERE tenant_id = $1 AND id = $2 FOR NO KEY UPDATE",
		tenant, id,
	).Scan(&locked)
	if err != nil {
		return fmt.Errorf("email: lock mailbox: %w", err)
	}
	return nil
}

// addUsedBytes adds size to the bytes the workspace tenant keeps, inside tx.
func addUsedBytes(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, size int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO plugin_email.workspace_usage (tenant_id, used_bytes) VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE
		SET used_bytes = plugin_email.workspace_usage.used_bytes + EXCLUDED.used_bytes`,
		tenant, size)
	if err != nil {
		return fmt.Errorf("email: add used bytes: %w", err)
	}
	return nil
}

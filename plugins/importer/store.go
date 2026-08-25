// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// The states an import moves through.
const (
	stateReady      = "ready"
	stateCommitting = "committing"
	stateCommitted  = "committed"
)

// The outcomes a staged row settles into.
const (
	outcomePending  = "pending"
	outcomeImported = "imported"
	outcomeSkipped  = "skipped"
	outcomeFailed   = "failed"
)

// mapping assigns a field to each mapped column, keyed by column index.
type mapping map[string]fieldName

// store persists imports and their staged rows.
type store struct {
	pool *pgxpool.Pool
}

type importRow struct {
	ID            uuid.UUID `db:"id"`
	UserID        uuid.UUID `db:"user_id"`
	Filename      string    `db:"filename"`
	State         string    `db:"state"`
	RowCount      int       `db:"row_count"`
	ImportedCount int       `db:"imported_count"`
	SkippedCount  int       `db:"skipped_count"`
	FailedCount   int       `db:"failed_count"`
	CreatedAt     time.Time `db:"created_at"`
	Columns       []string  `db:"columns"`
	Mapping       mapping   `db:"mapping"`
}

type stagedRow struct {
	ID        uuid.UUID  `db:"id"`
	Position  int        `db:"position"`
	Cells     []string   `db:"cells"`
	Outcome   string     `db:"outcome"`
	Reason    *string    `db:"reason"`
	ContactID *uuid.UUID `db:"contact_id"`
}

type importContactRow struct {
	ContactID uuid.UUID `db:"contact_id"`
	Name      string    `db:"name"`
	RowID     uuid.UUID `db:"row_id"`
}

// summaryColumns are the import columns the history list reads.
const summaryColumns = `id, user_id, filename, state, row_count, imported_count,
	skipped_count, failed_count, created_at`

// listImports returns every stored import, newest first.
func (s *store) listImports(ctx context.Context) ([]importRow, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT `+summaryColumns+`, columns, mapping
		FROM plugin_importer.imports
		WHERE tenant_id = $1
		ORDER BY created_at DESC, id DESC`, sdk.TenantOrDefault(ctx))
	imports, err := pgx.CollectRows(rows, pgx.RowToStructByName[importRow])
	if err != nil {
		return nil, fmt.Errorf("importer: list imports: %w", err)
	}
	return imports, nil
}

// importByID returns one stored import, or [pgx.ErrNoRows] when none carries the id.
func (s *store) importByID(ctx context.Context, id uuid.UUID) (importRow, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT `+summaryColumns+`, columns, mapping
		FROM plugin_importer.imports WHERE id = $1 AND tenant_id = $2`,
		id, sdk.TenantOrDefault(ctx))
	stored, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[importRow])
	if err != nil {
		return importRow{}, err
	}
	return stored, nil
}

// listRows returns up to limit staged rows of one import in position order.
func (s *store) listRows(ctx context.Context, importID uuid.UUID, limit int) ([]stagedRow, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT id, position, cells, outcome, reason, contact_id
		FROM plugin_importer.import_rows
		WHERE import_id = $1 AND tenant_id = $3
		ORDER BY position
		LIMIT $2`, importID, limit, sdk.TenantOrDefault(ctx))
	staged, err := pgx.CollectRows(rows, pgx.RowToStructByName[stagedRow])
	if err != nil {
		return nil, fmt.Errorf("importer: list rows: %w", err)
	}
	return staged, nil
}

// listImportContacts returns the contacts an import created, in row order.
func (s *store) listImportContacts(ctx context.Context, importID uuid.UUID) ([]importContactRow, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT c.id AS contact_id, c.name, r.id AS row_id
		FROM plugin_importer.import_rows r
		JOIN core.contacts c ON c.id = r.contact_id
		WHERE r.import_id = $1 AND r.outcome = $2 AND r.tenant_id = $3
		ORDER BY r.position`, importID, outcomeImported, sdk.TenantOrDefault(ctx))
	linked, err := pgx.CollectRows(rows, pgx.RowToStructByName[importContactRow])
	if err != nil {
		return nil, fmt.Errorf("importer: list import contacts: %w", err)
	}
	return linked, nil
}

type commitCounts struct {
	Imported int `db:"imported_count"`
	Skipped  int `db:"skipped_count"`
	Failed   int `db:"failed_count"`
}

// claimForCommit moves an import into committing and returns its mapping.
func (s *store) claimForCommit(ctx context.Context, id uuid.UUID) (mapping, error) {
	var claimed mapping
	var state string
	err := s.pool.QueryRow(ctx,
		`UPDATE plugin_importer.imports SET state = $2
		WHERE id = $1 AND state IN ($2, $3) AND tenant_id = $4
		RETURNING mapping, state`,
		id, stateCommitting, stateReady, sdk.TenantOrDefault(ctx)).Scan(&claimed, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errAlreadyCommitted
	}
	if err != nil {
		return nil, fmt.Errorf("importer: claim import: %w", err)
	}
	if len(claimed) == 0 {
		return nil, errNoMapping
	}
	return claimed, nil
}

// pendingRows returns the rows of an import still waiting to be committed.
func (s *store) pendingRows(ctx context.Context, importID uuid.UUID) ([]stagedRow, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT id, position, cells, outcome, reason, contact_id
		FROM plugin_importer.import_rows
		WHERE import_id = $1 AND outcome = $2 AND tenant_id = $3
		ORDER BY position`, importID, outcomePending, sdk.TenantOrDefault(ctx))
	pending, err := pgx.CollectRows(rows, pgx.RowToStructByName[stagedRow])
	if err != nil {
		return nil, fmt.Errorf("importer: list pending rows: %w", err)
	}
	return pending, nil
}

// settleRow records the outcome one staged row reached.
func (s *store) settleRow(ctx context.Context, rowID uuid.UUID, settled settlement) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE plugin_importer.import_rows
		SET outcome = $2, reason = COALESCE($3, reason), contact_id = $4
		WHERE id = $1 AND tenant_id = $5`,
		rowID, settled.outcome, optionalReason(settled.reason), settled.contactID,
		sdk.TenantOrDefault(ctx)); err != nil {
		return fmt.Errorf("importer: settle row: %w", err)
	}
	return nil
}

// finishCommit counts the settled rows, marks the import committed, and returns the counts.
func (s *store) finishCommit(ctx context.Context, id uuid.UUID) (commitCounts, error) {
	var counts commitCounts
	err := s.pool.QueryRow(ctx,
		`UPDATE plugin_importer.imports i SET state = $2,
			imported_count = tally.imported,
			skipped_count = tally.skipped,
			failed_count = tally.failed
		FROM (
			SELECT
				count(*) FILTER (WHERE outcome = $3) AS imported,
				count(*) FILTER (WHERE outcome = $4) AS skipped,
				count(*) FILTER (WHERE outcome = $5) AS failed
			FROM plugin_importer.import_rows WHERE import_id = $1 AND tenant_id = $6
		) tally
		WHERE i.id = $1 AND i.tenant_id = $6
		RETURNING i.imported_count, i.skipped_count, i.failed_count`,
		id, stateCommitted, outcomeImported, outcomeSkipped, outcomeFailed,
		sdk.TenantOrDefault(ctx),
	).Scan(&counts.Imported, &counts.Skipped, &counts.Failed)
	if err != nil {
		return commitCounts{}, fmt.Errorf("importer: finish commit: %w", err)
	}
	return counts, nil
}

// updateMapping stores the column assignments of an import that is still ready.
func (s *store) updateMapping(ctx context.Context, id uuid.UUID, assigned mapping) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE plugin_importer.imports SET mapping = $2
		WHERE id = $1 AND state = $3 AND tenant_id = $4`,
		id, assigned, stateReady, sdk.TenantOrDefault(ctx)); err != nil {
		return fmt.Errorf("importer: update mapping: %w", err)
	}
	return nil
}

// insertImport stores one parsed upload and its rows, returning the import id.
func (s *store) insertImport(
	ctx context.Context, uploader uuid.UUID, filename string, parsed sheet,
) (importRow, error) {
	ids, err := newIDs(len(parsed.rows) + 1)
	if err != nil {
		return importRow{}, fmt.Errorf("importer: generate id: %w", err)
	}
	stored, err := s.writeImport(ctx, ids, uploader, filename, parsed)
	if err != nil {
		return importRow{}, fmt.Errorf("importer: store import: %w", err)
	}
	return stored, nil
}

// newIDs returns count freshly generated identifiers.
func newIDs(count int) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, count)
	for i := range ids {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

// writeImport inserts the import and every staged row in one transaction.
func (s *store) writeImport(
	ctx context.Context, ids []uuid.UUID, uploader uuid.UUID, filename string, parsed sheet,
) (importRow, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return importRow{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, _ := tx.Query(ctx,
		"INSERT INTO plugin_importer.imports (id, user_id, filename, columns, mapping, "+
			"state, row_count, imported_count, skipped_count, failed_count, created_at, tenant_id) "+
			"VALUES ($1, $2, $3, $4, '{}', $5, $6, 0, 0, 0, now(), $7) "+
			"RETURNING "+summaryColumns+", columns, mapping",
		ids[0], uploader, filename, parsed.columns, stateReady, len(parsed.rows),
		sdk.TenantOrDefault(ctx),
	)
	stored, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[importRow])
	if err != nil {
		return importRow{}, err
	}
	if err := insertRows(ctx, tx, ids, parsed.rows); err != nil {
		return importRow{}, err
	}
	return stored, tx.Commit(ctx)
}

// insertRows stores every staged row of an import through tx.
func insertRows(ctx context.Context, tx pgx.Tx, ids []uuid.UUID, rows []row) error {
	for i, stored := range rows {
		if _, err := tx.Exec(ctx,
			"INSERT INTO plugin_importer.import_rows (id, import_id, position, cells, "+
				"outcome, reason, tenant_id) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			ids[i+1], ids[0], i+1, stored.cells, outcomePending, optionalReason(stored.reason),
			sdk.TenantOrDefault(ctx),
		); err != nil {
			return err
		}
	}
	return nil
}

// optionalReason returns the note a row carries, or nil when it needed no repair.
func optionalReason(reason string) *string {
	if reason == "" {
		return nil
	}
	return &reason
}

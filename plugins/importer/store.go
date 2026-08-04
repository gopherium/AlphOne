// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// stateReady marks an import parsed and waiting to be mapped.
const stateReady = "ready"

// The outcomes a staged row settles into.
const (
	outcomePending  = "pending"
	outcomeImported = "imported"
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
		ORDER BY created_at DESC, id DESC`)
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
		FROM plugin_importer.imports WHERE id = $1`, id)
	stored, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[importRow])
	if err != nil {
		return importRow{}, err
	}
	return stored, nil
}

// listRows returns the staged rows of one import in position order.
func (s *store) listRows(ctx context.Context, importID uuid.UUID) ([]stagedRow, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT id, position, cells, outcome, reason, contact_id
		FROM plugin_importer.import_rows
		WHERE import_id = $1
		ORDER BY position`, importID)
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
		WHERE r.import_id = $1 AND r.outcome = $2
		ORDER BY r.position`, importID, outcomeImported)
	linked, err := pgx.CollectRows(rows, pgx.RowToStructByName[importContactRow])
	if err != nil {
		return nil, fmt.Errorf("importer: list import contacts: %w", err)
	}
	return linked, nil
}

// updateMapping stores the column assignments of an import that is still ready.
func (s *store) updateMapping(ctx context.Context, id uuid.UUID, assigned mapping) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE plugin_importer.imports SET mapping = $2 WHERE id = $1 AND state = $3`,
		id, assigned, stateReady); err != nil {
		return fmt.Errorf("importer: update mapping: %w", err)
	}
	return nil
}

// insertImport stores one parsed upload and its rows, returning the import id.
func (s *store) insertImport(
	ctx context.Context, uploader uuid.UUID, filename string, parsed sheet,
) (uuid.UUID, error) {
	ids, err := newIDs(len(parsed.rows) + 1)
	if err != nil {
		return uuid.Nil, fmt.Errorf("importer: generate id: %w", err)
	}
	if err := s.writeImport(ctx, ids, uploader, filename, parsed); err != nil {
		return uuid.Nil, fmt.Errorf("importer: store import: %w", err)
	}
	return ids[0], nil
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
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		"INSERT INTO plugin_importer.imports (id, user_id, filename, columns, mapping, "+
			"state, row_count, imported_count, skipped_count, failed_count, created_at) "+
			"VALUES ($1, $2, $3, $4, '{}', $5, $6, 0, 0, 0, now())",
		ids[0], uploader, filename, parsed.columns, stateReady, len(parsed.rows),
	); err != nil {
		return err
	}
	if err := insertRows(ctx, tx, ids, parsed.rows); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// insertRows stores every staged row of an import through tx.
func insertRows(ctx context.Context, tx pgx.Tx, ids []uuid.UUID, rows []row) error {
	for i, stored := range rows {
		if _, err := tx.Exec(ctx,
			"INSERT INTO plugin_importer.import_rows (id, import_id, position, cells, "+
				"outcome, reason) VALUES ($1, $2, $3, $4, $5, $6)",
			ids[i+1], ids[0], i+1, stored.cells, outcomePending, optionalReason(stored.reason),
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

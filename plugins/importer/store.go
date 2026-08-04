// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// stateReady marks an import parsed and waiting to be mapped.
const stateReady = "ready"

// outcomePending marks a row parsed but not yet committed.
const outcomePending = "pending"

// store persists imports and their staged rows.
type store struct {
	pool *pgxpool.Pool
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

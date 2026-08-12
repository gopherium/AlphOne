// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store errors.
var (
	errNameTaken    = errors.New("fields: another definition holds that name")
	errNoDefinition = errors.New("fields: no live definition holds that id")
)

// uniqueViolation is the Postgres code a duplicate name raises.
const uniqueViolation = "23505"

// store reads and writes the definition catalogue.
type store struct {
	pool *pgxpool.Pool
}

// create stores a definition, refusing a name another already holds.
func (s *store) create(ctx context.Context, definition Definition) error {
	const statement = `INSERT INTO plugin_fields.definitions (id, name, label, kind, created_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := s.pool.Exec(ctx, statement,
		definition.ID, definition.Name, definition.Label, string(definition.Kind), definition.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return errNameTaken
	}
	if err != nil {
		return fmt.Errorf("fields: create definition: %w", err)
	}
	return nil
}

// archive marks a live definition archived, refusing an id no live row holds.
func (s *store) archive(ctx context.Context, id uuid.UUID) error {
	const statement = `UPDATE plugin_fields.definitions SET archived_at = now()
		WHERE id = $1 AND archived_at IS NULL`
	tag, err := s.pool.Exec(ctx, statement, id)
	if err != nil {
		return fmt.Errorf("fields: archive definition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errNoDefinition
	}
	return nil
}

// liveDefinitions lists every definition no operator has archived.
func (s *store) liveDefinitions(ctx context.Context) ([]Definition, error) {
	const query = `SELECT id, name, label, kind, archived_at, created_at
		FROM plugin_fields.definitions WHERE archived_at IS NULL ORDER BY created_at, id`
	return s.query(ctx, query)
}

// allDefinitions lists every definition, archived ones included.
func (s *store) allDefinitions(ctx context.Context) ([]Definition, error) {
	const query = `SELECT id, name, label, kind, archived_at, created_at
		FROM plugin_fields.definitions ORDER BY created_at, id`
	return s.query(ctx, query)
}

// query reads the definitions the given statement selects.
func (s *store) query(ctx context.Context, statement string) ([]Definition, error) {
	rows, err := s.pool.Query(ctx, statement)
	if err != nil {
		return nil, fmt.Errorf("fields: list definitions: %w", err)
	}
	defer rows.Close()
	held, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Definition, error) {
		var definition Definition
		var declared string
		err := row.Scan(&definition.ID, &definition.Name, &definition.Label,
			&declared, &definition.ArchivedAt, &definition.CreatedAt)
		definition.Kind = kind(declared)
		return definition, err
	})
	if err != nil {
		return nil, fmt.Errorf("fields: read definitions: %w", err)
	}
	return held, nil
}

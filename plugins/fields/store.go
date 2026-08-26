// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// Store errors.
var (
	errNameTaken    = errors.New("fields: another definition holds that name")
	errNoDefinition = errors.New("fields: no live definition holds that id")
	errKindLocked   = errors.New("fields: the archived definition of that name holds another kind")
)

// store reads and writes the definition catalogue.
type store struct {
	pool *pgxpool.Pool
}

// define stores a definition, reviving an archived one of the same name and kind.
func (s *store) define(ctx context.Context, definition Definition) error {
	const statement = `INSERT INTO plugin_fields.definitions
			(id, name, label, kind, created_at, tenant_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, name) DO UPDATE SET archived_at = NULL, label = EXCLUDED.label
		WHERE plugin_fields.definitions.archived_at IS NOT NULL
			AND plugin_fields.definitions.kind = EXCLUDED.kind`
	tag, err := s.pool.Exec(ctx, statement,
		definition.ID, definition.Name, definition.Label, string(definition.Kind),
		definition.CreatedAt, sdk.TenantOrDefault(ctx))
	if err != nil {
		return fmt.Errorf("fields: define definition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return s.errorFor(ctx, definition)
	}
	return nil
}

// errorFor reports why a definition the store refused could not be written.
func (s *store) errorFor(ctx context.Context, definition Definition) error {
	const query = `SELECT archived_at IS NULL FROM plugin_fields.definitions
		WHERE name = $1 AND tenant_id = $2`
	var live bool
	if err := s.pool.QueryRow(ctx, query, definition.Name, sdk.TenantOrDefault(ctx)).Scan(&live); err != nil {
		return fmt.Errorf("fields: read the held definition: %w", err)
	}
	if live {
		return errNameTaken
	}
	return errKindLocked
}

// archive marks a live definition archived.
func (s *store) archive(ctx context.Context, id uuid.UUID) error {
	const statement = `UPDATE plugin_fields.definitions SET archived_at = now()
		WHERE id = $1 AND archived_at IS NULL AND tenant_id = $2`
	tag, err := s.pool.Exec(ctx, statement, id, sdk.TenantOrDefault(ctx))
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
		FROM plugin_fields.definitions
		WHERE archived_at IS NULL AND tenant_id = $1 ORDER BY created_at, id`
	return s.query(ctx, query)
}

// allDefinitions lists every definition, archived ones included.
func (s *store) allDefinitions(ctx context.Context) ([]Definition, error) {
	const query = `SELECT id, name, label, kind, archived_at, created_at
		FROM plugin_fields.definitions WHERE tenant_id = $1 ORDER BY created_at, id`
	return s.query(ctx, query)
}

// writeValues merges values into a contact's bag, dropping the keys written null.
func (s *store) writeValues(ctx context.Context, contactID uuid.UUID, values map[string]any) error {
	const statement = `INSERT INTO plugin_fields.contact_values (contact_id, values, tenant_id)
		VALUES ($1, jsonb_strip_nulls($2::jsonb), $3)
		ON CONFLICT (tenant_id, contact_id) DO UPDATE
		SET values = jsonb_strip_nulls(plugin_fields.contact_values.values || $2::jsonb)`
	if _, err := s.pool.Exec(ctx, statement, contactID, values, sdk.TenantOrDefault(ctx)); err != nil {
		return fmt.Errorf("fields: write contact values: %w", err)
	}
	return nil
}

// valueRow pairs a contact with the value bag it holds.
type valueRow struct {
	contactID uuid.UUID
	values    map[string]any
}

// valuesFor reads the value bags of the given contacts.
func (s *store) valuesFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]map[string]any, error) {
	const query = `SELECT contact_id, values FROM plugin_fields.contact_values
		WHERE contact_id = ANY($1) AND tenant_id = $2`
	return s.collectValues(ctx, query, ids)
}

// collectValues reads the value bags the given statement selects.
func (s *store) collectValues(
	ctx context.Context, statement string, ids []uuid.UUID,
) (map[uuid.UUID]map[string]any, error) {
	rows, err := s.pool.Query(ctx, statement, ids, sdk.TenantOrDefault(ctx))
	if err != nil {
		return nil, fmt.Errorf("fields: read contact values: %w", err)
	}
	defer rows.Close()
	collected, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (valueRow, error) {
		var held valueRow
		return held, row.Scan(&held.contactID, &held.values)
	})
	if err != nil {
		return nil, fmt.Errorf("fields: read one contact's values: %w", err)
	}
	held := make(map[uuid.UUID]map[string]any, len(collected))
	for _, row := range collected {
		held[row.contactID] = row.values
	}
	return held, nil
}

// query reads the definitions the given statement selects.
func (s *store) query(ctx context.Context, statement string) ([]Definition, error) {
	rows, err := s.pool.Query(ctx, statement, sdk.TenantOrDefault(ctx))
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

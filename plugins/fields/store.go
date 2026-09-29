// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// Store errors.
var (
	errNameTaken       = errors.New("fields: another definition holds that name")
	errNoDefinition    = errors.New("fields: no live definition holds that id")
	errKindLocked      = errors.New("fields: the archived definition of that name holds another kind or other sub fields")
	errNoContact       = errors.New("fields: the caller's tenant holds no contact of that id")
	errEntriesFull     = errors.New("fields: the repeater has no room for the entries given")
	errNoEntry         = errors.New("fields: no entry of that id is in that contact's field")
	errOrderIncomplete = errors.New("fields: the order does not name each live field exactly once")
)

// store reads and writes the definition catalogue.
type store struct {
	pool *pgxpool.Pool
}

// define stores a definition, reviving an archived one of the same shape, and answers the id the stored row keeps.
func (s *store) define(ctx context.Context, definition Definition) (uuid.UUID, error) {
	const statement = `WITH stored AS (
			INSERT INTO plugin_fields.definitions
				(id, name, label, kind, created_at, tenant_id, sub_fields)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
			ON CONFLICT (tenant_id, name) DO UPDATE
				SET archived_at = NULL, label = EXCLUDED.label, sub_fields = EXCLUDED.sub_fields,
					position = nextval('plugin_fields.definitions_position_seq')
			WHERE plugin_fields.definitions.archived_at IS NOT NULL
				AND plugin_fields.definitions.kind = EXCLUDED.kind
				AND jsonb_path_query_array(plugin_fields.definitions.sub_fields, '$[*].name')
					= jsonb_path_query_array(EXCLUDED.sub_fields, '$[*].name')
				AND jsonb_path_query_array(plugin_fields.definitions.sub_fields, '$[*].kind')
					= jsonb_path_query_array(EXCLUDED.sub_fields, '$[*].kind')
			RETURNING id
		), swept AS (
			UPDATE plugin_fields.contact_values SET values = values - $2::text
			WHERE tenant_id = $6 AND values ? $2::text
				AND EXISTS (SELECT 1 FROM stored)
				AND NOT EXISTS (SELECT 1 FROM plugin_fields.definitions WHERE tenant_id = $6 AND name = $2)
		)
		SELECT id FROM stored`
	var stored uuid.UUID
	err := s.pool.QueryRow(ctx, statement,
		definition.ID, definition.Name, definition.Label, string(definition.Kind),
		definition.CreatedAt, sdk.TenantOrDefault(ctx),
		append([]SubField{}, definition.SubFields...)).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, s.errorFor(ctx, definition)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("fields: define definition: %w", err)
	}
	return stored, nil
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
	const query = `SELECT id, name, label, kind, sub_fields, archived_at, created_at
		FROM plugin_fields.definitions
		WHERE archived_at IS NULL AND tenant_id = $1 ORDER BY position, id`
	return s.query(ctx, query)
}

// allDefinitions lists every definition, the live ones first in their order and the archived ones after.
func (s *store) allDefinitions(ctx context.Context) ([]Definition, error) {
	const query = `SELECT id, name, label, kind, sub_fields, archived_at, created_at
		FROM plugin_fields.definitions WHERE tenant_id = $1 ORDER BY archived_at IS NOT NULL, position, id`
	return s.query(ctx, query)
}

// lockLiveStatement locks the tenant's live definitions by id and answers their ids.
const lockLiveStatement = `SELECT array_agg(id) FROM (
		SELECT id FROM plugin_fields.definitions
		WHERE tenant_id = $1 AND archived_at IS NULL ORDER BY id FOR UPDATE
	) AS locked`

// orderStatement sets the position of each named definition to its place in the list.
const orderStatement = `UPDATE plugin_fields.definitions AS d SET position = o.n
	FROM unnest($2::uuid[]) WITH ORDINALITY AS o (id, n)
	WHERE d.id = o.id AND d.tenant_id = $1`

// order sets live positions from ids, refusing a missing or repeated id or an archived, unknown or other tenant's id.
func (s *store) order(ctx context.Context, ids []uuid.UUID) error {
	tenant := sdk.TenantOrDefault(ctx)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var held []uuid.UUID
		if err := tx.QueryRow(ctx, lockLiveStatement, tenant).Scan(&held); err != nil {
			return err
		}
		if !namesEachOnce(ids, held) {
			return errOrderIncomplete
		}
		_, err := tx.Exec(ctx, orderStatement, tenant, ids)
		return err
	})
	if err != nil && !errors.Is(err, errOrderIncomplete) {
		return fmt.Errorf("fields: order definitions: %w", err)
	}
	return err
}

// namesEachOnce reports whether given names every held id exactly once and nothing else.
func namesEachOnce(given, held []uuid.UUID) bool {
	left := make(map[uuid.UUID]bool, len(held))
	for _, id := range held {
		left[id] = true
	}
	for _, id := range given {
		if !left[id] {
			return false
		}
		delete(left, id)
	}
	return len(left) == 0
}

// writeValues merges values into a contact's field values, dropping the keys written null.
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

// contactReference is the constraint tying a value row to the contact it describes.
const contactReference = "contact_values_contact_id_fkey"

// addEntryStatement prepends entries to a contact's list and says whether the contact and the room were there.
const addEntryStatement = `WITH entry_contact AS (
		SELECT id, tenant_id FROM core.contacts WHERE id = $1 AND tenant_id = $4
	), added AS (
		INSERT INTO plugin_fields.contact_values (contact_id, values, tenant_id)
		SELECT id, jsonb_build_object($2::text, $3::jsonb), tenant_id FROM entry_contact
		WHERE jsonb_array_length($3::jsonb) <= $5
		ON CONFLICT (tenant_id, contact_id) DO UPDATE
		SET values = plugin_fields.contact_values.values || jsonb_build_object($2::text,
			$3::jsonb || coalesce(plugin_fields.contact_values.values -> $2::text, '[]'::jsonb))
		WHERE jsonb_array_length(coalesce(plugin_fields.contact_values.values -> $2::text, '[]'::jsonb))
			+ jsonb_array_length($3::jsonb) <= $5
		RETURNING 1
	)
	SELECT EXISTS (SELECT 1 FROM entry_contact), EXISTS (SELECT 1 FROM added)`

// updateEntryStatement replaces the cells of one entry in place, keeping its id.
const updateEntryStatement = `UPDATE plugin_fields.contact_values
	SET values = values || jsonb_build_object($2::text, (
		SELECT jsonb_agg(CASE WHEN listed.entry ->> 'id' = $3::text
			THEN $4::jsonb || jsonb_build_object('id', $3::text) ELSE listed.entry END ORDER BY listed.at)
		FROM jsonb_array_elements(values -> $2::text) WITH ORDINALITY AS listed (entry, at)))
	WHERE tenant_id = $5 AND contact_id = $1
		AND values -> $2::text @> jsonb_build_array(jsonb_build_object('id', $3::text))`

// deleteEntryStatement drops one entry, and the key with the last one.
const deleteEntryStatement = `UPDATE plugin_fields.contact_values
	SET values = (values - $2::text) || coalesce((
		SELECT jsonb_build_object($2::text, jsonb_agg(listed.entry ORDER BY listed.at))
		FROM jsonb_array_elements(values -> $2::text) WITH ORDINALITY AS listed (entry, at)
		WHERE listed.entry ->> 'id' IS DISTINCT FROM $3::text
		HAVING count(*) > 0), '{}'::jsonb)
	WHERE tenant_id = $4 AND contact_id = $1
		AND values -> $2::text @> jsonb_build_array(jsonb_build_object('id', $3::text))`

// addEntry prepends entries to a contact's repeater list, the last given on top, and returns them with their ids.
func (s *store) addEntry(
	ctx context.Context, contactID uuid.UUID, name string, entries []map[string]any, limit int,
) ([]map[string]any, error) {
	stored := newestFirst(entries)
	var held, added bool
	err := s.pool.QueryRow(ctx, addEntryStatement,
		contactID, name, stored, sdk.TenantOrDefault(ctx), limit).Scan(&held, &added)
	switch {
	case isMissingContact(err):
		return nil, errNoContact
	case err != nil:
		return nil, fmt.Errorf("fields: add entries: %w", err)
	case !held:
		return nil, errNoContact
	case !added:
		return nil, errEntriesFull
	}
	return stored, nil
}

// newestFirst returns the entries each under a fresh id, the last given first.
func newestFirst(entries []map[string]any) []map[string]any {
	stored := make([]map[string]any, len(entries))
	for at, cells := range entries {
		entry := maps.Clone(cells)
		entry[entryIDKey] = uuid.Must(uuid.NewV7()).String()
		stored[len(entries)-1-at] = entry
	}
	return stored
}

// isMissingContact reports whether err is a value row naming a contact that no longer exists.
func isMissingContact(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == contactReference
}

// updateEntry replaces the cells of one entry in a contact's repeater list, keeping its id and its place.
func (s *store) updateEntry(
	ctx context.Context, contactID uuid.UUID, name string, id uuid.UUID, cells map[string]any,
) error {
	tag, err := s.pool.Exec(ctx, updateEntryStatement,
		contactID, name, id.String(), cells, sdk.TenantOrDefault(ctx))
	if err != nil {
		return fmt.Errorf("fields: update entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errNoEntry
	}
	return nil
}

// deleteEntry drops one entry from a contact's repeater list, dropping the field with its last entry.
func (s *store) deleteEntry(ctx context.Context, contactID uuid.UUID, name string, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, deleteEntryStatement, contactID, name, id.String(), sdk.TenantOrDefault(ctx))
	if err != nil {
		return fmt.Errorf("fields: delete entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errNoEntry
	}
	return nil
}

// valueRow pairs a contact with the field values it holds.
type valueRow struct {
	contactID uuid.UUID
	values    map[string]any
}

// valuesFor reads the field values of the given contacts.
func (s *store) valuesFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]map[string]any, error) {
	const query = `SELECT contact_id, values FROM plugin_fields.contact_values
		WHERE contact_id = ANY($1) AND tenant_id = $2`
	return s.collectValues(ctx, query, ids)
}

// collectValues reads the field values the given statement selects.
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
			&declared, &definition.SubFields, &definition.ArchivedAt, &definition.CreatedAt)
		definition.Kind = kind(declared)
		return definition, err
	})
	if err != nil {
		return nil, fmt.Errorf("fields: read definitions: %w", err)
	}
	return held, nil
}

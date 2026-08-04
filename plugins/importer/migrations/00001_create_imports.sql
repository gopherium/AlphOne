-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE plugin_importer.imports (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    filename text NOT NULL,
    columns text[] NOT NULL,
    mapping jsonb NOT NULL CHECK (jsonb_typeof(mapping) = 'object'),
    state text NOT NULL CHECK (state IN ('ready', 'committing', 'committed')),
    row_count integer NOT NULL,
    imported_count integer NOT NULL,
    skipped_count integer NOT NULL,
    failed_count integer NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE plugin_importer.import_rows (
    id uuid PRIMARY KEY,
    import_id uuid NOT NULL REFERENCES plugin_importer.imports (id) ON DELETE CASCADE,
    position integer NOT NULL,
    cells jsonb NOT NULL CHECK (jsonb_typeof(cells) = 'array'),
    outcome text NOT NULL CHECK (outcome IN ('pending', 'imported', 'skipped', 'failed')),
    reason text,
    contact_id uuid REFERENCES core.contacts (id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX import_rows_import_position_idx
    ON plugin_importer.import_rows (import_id, position);

-- +goose Down
DROP TABLE plugin_importer.import_rows;
DROP TABLE plugin_importer.imports;

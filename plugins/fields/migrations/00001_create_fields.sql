-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE plugin_fields.definitions (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE,
    label text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('TEXT', 'LONGTEXT', 'NUMBER', 'BOOLEAN', 'DATE', 'SELECT')),
    archived_at timestamptz,
    created_at timestamptz NOT NULL
);

CREATE INDEX definitions_live_idx ON plugin_fields.definitions (name) WHERE archived_at IS NULL;

-- +goose Down
DROP TABLE plugin_fields.definitions;

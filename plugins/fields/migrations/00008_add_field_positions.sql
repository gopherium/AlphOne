-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE SEQUENCE plugin_fields.definitions_position_seq;
ALTER TABLE plugin_fields.definitions ADD COLUMN position bigint;

UPDATE plugin_fields.definitions AS d
SET position = ranked.n
FROM (
    SELECT id, row_number() OVER (ORDER BY created_at, id) AS n FROM plugin_fields.definitions
) AS ranked
WHERE d.id = ranked.id;

SELECT setval('plugin_fields.definitions_position_seq',
    coalesce((SELECT max(position) FROM plugin_fields.definitions), 0) + 1, false);

ALTER TABLE plugin_fields.definitions
    ALTER COLUMN position SET DEFAULT nextval('plugin_fields.definitions_position_seq'),
    ALTER COLUMN position SET NOT NULL;
ALTER SEQUENCE plugin_fields.definitions_position_seq OWNED BY plugin_fields.definitions.position;

-- +goose Down
ALTER TABLE plugin_fields.definitions DROP COLUMN position;

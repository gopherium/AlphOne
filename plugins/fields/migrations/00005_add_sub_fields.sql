-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE plugin_fields.definitions DROP CONSTRAINT definitions_kind_check;
ALTER TABLE plugin_fields.definitions ADD CONSTRAINT definitions_kind_check
    CHECK (kind IN ('TEXT', 'LONGTEXT', 'NUMBER', 'BOOLEAN', 'DATE', 'SELECT', 'REPEATER'));
ALTER TABLE plugin_fields.definitions
    ADD COLUMN sub_fields jsonb NOT NULL DEFAULT '[]'
        CONSTRAINT definitions_sub_fields_array CHECK (jsonb_typeof(sub_fields) = 'array');
ALTER TABLE plugin_fields.definitions ADD CONSTRAINT definitions_sub_fields_repeater
    CHECK ((kind = 'REPEATER') = (jsonb_array_length(sub_fields) > 0));

-- +goose Down
ALTER TABLE plugin_fields.definitions DROP CONSTRAINT definitions_sub_fields_repeater;
ALTER TABLE plugin_fields.definitions DROP COLUMN sub_fields;
ALTER TABLE plugin_fields.definitions DROP CONSTRAINT definitions_kind_check;
ALTER TABLE plugin_fields.definitions ADD CONSTRAINT definitions_kind_check
    CHECK (kind IN ('TEXT', 'LONGTEXT', 'NUMBER', 'BOOLEAN', 'DATE', 'SELECT'));

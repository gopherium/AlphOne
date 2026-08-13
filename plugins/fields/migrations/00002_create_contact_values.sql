-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE plugin_fields.contact_values (
    contact_id uuid PRIMARY KEY REFERENCES core.contacts (id) ON DELETE CASCADE,
    values jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(values) = 'object')
);

-- +goose Down
DROP TABLE plugin_fields.contact_values;

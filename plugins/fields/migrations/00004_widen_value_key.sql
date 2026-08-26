-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE plugin_fields.contact_values DROP CONSTRAINT contact_values_pkey;
ALTER TABLE plugin_fields.contact_values ADD CONSTRAINT contact_values_pkey
    PRIMARY KEY (tenant_id, contact_id);

-- +goose Down
ALTER TABLE plugin_fields.contact_values DROP CONSTRAINT contact_values_pkey;
ALTER TABLE plugin_fields.contact_values ADD CONSTRAINT contact_values_pkey
    PRIMARY KEY (contact_id);

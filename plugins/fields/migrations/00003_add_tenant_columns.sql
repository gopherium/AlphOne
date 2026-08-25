-- SPDX-License-Identifier: AGPL-3.0-or-later

-- +goose Up
ALTER TABLE plugin_fields.definitions ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE plugin_fields.contact_values ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE plugin_fields.definitions DROP CONSTRAINT definitions_name_key;
ALTER TABLE plugin_fields.definitions
    ADD CONSTRAINT definitions_tenant_name_key UNIQUE (tenant_id, name);

-- +goose Down
ALTER TABLE plugin_fields.definitions DROP CONSTRAINT definitions_tenant_name_key;
ALTER TABLE plugin_fields.definitions ADD CONSTRAINT definitions_name_key UNIQUE (name);
ALTER TABLE plugin_fields.contact_values DROP COLUMN tenant_id;
ALTER TABLE plugin_fields.definitions DROP COLUMN tenant_id;

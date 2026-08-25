-- SPDX-License-Identifier: AGPL-3.0-or-later

-- +goose Up
ALTER TABLE plugin_importer.imports ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE plugin_importer.import_rows ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);

-- +goose Down
ALTER TABLE plugin_importer.import_rows DROP COLUMN tenant_id;
ALTER TABLE plugin_importer.imports DROP COLUMN tenant_id;

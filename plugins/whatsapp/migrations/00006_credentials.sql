-- SPDX-License-Identifier: AGPL-3.0-or-later

-- +goose Up
CREATE TABLE plugin_whatsapp.credentials (
    tenant_id uuid PRIMARY KEY REFERENCES core.tenants (id),
    phone_number_id text NOT NULL UNIQUE,
    access_token bytea NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE plugin_whatsapp.credentials;

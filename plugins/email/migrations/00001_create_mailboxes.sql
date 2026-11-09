-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE plugin_email.mailboxes (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL UNIQUE REFERENCES core.tenants (id),
    address text NOT NULL,
    aliases text[] NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE plugin_email.workspace_usage (
    tenant_id uuid PRIMARY KEY REFERENCES core.tenants (id),
    used_bytes bigint NOT NULL
);

-- +goose Down
DROP TABLE plugin_email.workspace_usage;
DROP TABLE plugin_email.mailboxes;

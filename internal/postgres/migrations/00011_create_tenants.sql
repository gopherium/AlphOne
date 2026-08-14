-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE core.tenants (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.tenant_members (
    user_id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO core.tenants (id, name)
VALUES ('00000000-0000-7000-8000-000000000001', 'Default')
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE core.tenant_members;

DROP TABLE core.tenants;

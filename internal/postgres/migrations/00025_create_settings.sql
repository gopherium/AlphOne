-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE core.settings (
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    key text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_-]*(\.[a-z0-9_-]+)*$'),
    value text,
    encrypted bytea,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, key),
    CHECK ((value IS NULL) <> (encrypted IS NULL))
);

-- +goose Down
DROP TABLE core.settings;

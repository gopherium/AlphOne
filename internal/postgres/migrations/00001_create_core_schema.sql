-- SPDX-License-Identifier: AGPL-3.0-or-later

-- +goose Up
CREATE SCHEMA core;

CREATE TABLE core.contacts (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE core.contact_identities (
    id uuid PRIMARY KEY,
    contact_id uuid NOT NULL REFERENCES core.contacts (id) ON DELETE CASCADE,
    channel text NOT NULL,
    identifier text NOT NULL,
    display_name text NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (channel, identifier)
);

CREATE INDEX contact_identities_contact_id_idx ON core.contact_identities (contact_id);

-- +goose Down
DROP SCHEMA core CASCADE;

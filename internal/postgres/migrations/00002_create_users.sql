-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE core.users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE,
    name text NOT NULL,
    password_hash text NOT NULL,
    disabled boolean NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE core.sessions (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES core.users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON core.sessions (user_id);

-- +goose Down
DROP TABLE core.sessions;
DROP TABLE core.users;

-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE core.api_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    name text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    last_used_at timestamptz
);

CREATE INDEX api_tokens_user_created_idx ON core.api_tokens (user_id, created_at DESC, id);

-- +goose Down
DROP TABLE core.api_tokens;

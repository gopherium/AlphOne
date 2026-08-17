-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE core.api_tokens
    ADD COLUMN scopes text NOT NULL DEFAULT '*',
    ADD COLUMN expires_at timestamptz;

ALTER TABLE core.api_tokens
    ALTER COLUMN scopes DROP DEFAULT;

-- +goose Down
DELETE FROM core.api_tokens;

ALTER TABLE core.api_tokens
    DROP COLUMN scopes,
    DROP COLUMN expires_at;

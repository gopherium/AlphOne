-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE INDEX sessions_expires_at_idx ON core.sessions (expires_at);

-- +goose Down
DROP INDEX core.sessions_expires_at_idx;

-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE INDEX threads_last_message_idx ON plugin_email.threads (tenant_id, last_message_at DESC, id DESC);

-- +goose Down
DROP INDEX plugin_email.threads_last_message_idx;

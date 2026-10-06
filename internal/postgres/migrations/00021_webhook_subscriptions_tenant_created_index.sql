-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE INDEX webhook_subscriptions_tenant_created_id_idx
    ON core.webhook_subscriptions (tenant_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX core.webhook_subscriptions_tenant_created_id_idx;

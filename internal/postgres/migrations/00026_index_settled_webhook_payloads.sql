-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE INDEX webhook_deliveries_settled_payload_idx
    ON core.webhook_deliveries (deliver_after, id)
    WHERE status <> 'pending' AND payload <> '';

-- +goose Down
DROP INDEX core.webhook_deliveries_settled_payload_idx;

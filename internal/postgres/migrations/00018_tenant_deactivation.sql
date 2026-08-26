-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE core.tenants ADD COLUMN deactivated_at timestamptz;

-- +goose Down
ALTER TABLE core.tenants DROP COLUMN deactivated_at;

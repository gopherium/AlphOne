-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE INDEX contacts_tenant_created_id_idx ON core.contacts (tenant_id, created_at, id);

-- +goose Down
DROP INDEX core.contacts_tenant_created_id_idx;

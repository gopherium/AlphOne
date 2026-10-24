-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE core.contact_identities ADD COLUMN added_xid xid8 NOT NULL DEFAULT '0';
ALTER TABLE core.contact_identities ALTER COLUMN added_xid SET DEFAULT pg_current_xact_id();
ALTER TABLE core.contact_identities ADD COLUMN added_server bigint NOT NULL DEFAULT 0;
ALTER TABLE core.contact_identities ALTER COLUMN added_server SET DEFAULT (pg_control_system()).system_identifier;
CREATE INDEX contact_identities_tenant_channel_added_id_idx
    ON core.contact_identities (tenant_id, channel, added_xid, id);

-- +goose Down
DROP INDEX core.contact_identities_tenant_channel_added_id_idx;
ALTER TABLE core.contact_identities DROP COLUMN added_server;
ALTER TABLE core.contact_identities DROP COLUMN added_xid;

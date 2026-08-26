-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE plugin_whatsapp.conversations ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE plugin_whatsapp.messages ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE plugin_whatsapp.media ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE plugin_whatsapp.conversations
    DROP CONSTRAINT conversations_external_id_key;
ALTER TABLE plugin_whatsapp.conversations
    ADD CONSTRAINT conversations_tenant_external_id_key UNIQUE (tenant_id, external_id);
ALTER TABLE plugin_whatsapp.messages
    DROP CONSTRAINT messages_external_id_key;
ALTER TABLE plugin_whatsapp.messages
    ADD CONSTRAINT messages_tenant_external_id_key UNIQUE (tenant_id, external_id);

-- +goose Down
ALTER TABLE plugin_whatsapp.messages DROP CONSTRAINT messages_tenant_external_id_key;
ALTER TABLE plugin_whatsapp.messages
    ADD CONSTRAINT messages_external_id_key UNIQUE (external_id);
ALTER TABLE plugin_whatsapp.conversations DROP CONSTRAINT conversations_tenant_external_id_key;
ALTER TABLE plugin_whatsapp.conversations
    ADD CONSTRAINT conversations_external_id_key UNIQUE (external_id);
ALTER TABLE plugin_whatsapp.media DROP COLUMN tenant_id;
ALTER TABLE plugin_whatsapp.messages DROP COLUMN tenant_id;
ALTER TABLE plugin_whatsapp.conversations DROP COLUMN tenant_id;

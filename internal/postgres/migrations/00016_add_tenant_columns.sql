-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE core.tenants ADD CONSTRAINT tenants_name_key UNIQUE (name);
ALTER TABLE core.contacts ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE core.contact_identities ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE core.tasks ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE core.api_tokens ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE core.webhook_subscriptions ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE core.webhook_deliveries ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE core.user_settings ADD COLUMN tenant_id uuid NOT NULL
    DEFAULT '00000000-0000-7000-8000-000000000001' REFERENCES core.tenants (id);
ALTER TABLE core.contact_identities
    DROP CONSTRAINT contact_identities_channel_identifier_key;
ALTER TABLE core.contact_identities
    ADD CONSTRAINT contact_identities_tenant_channel_identifier_key
    UNIQUE (tenant_id, channel, identifier);

-- +goose Down
ALTER TABLE core.contact_identities
    DROP CONSTRAINT contact_identities_tenant_channel_identifier_key;
ALTER TABLE core.contact_identities
    ADD CONSTRAINT contact_identities_channel_identifier_key UNIQUE (channel, identifier);
ALTER TABLE core.user_settings DROP COLUMN tenant_id;
ALTER TABLE core.webhook_deliveries DROP COLUMN tenant_id;
ALTER TABLE core.webhook_subscriptions DROP COLUMN tenant_id;
ALTER TABLE core.api_tokens DROP COLUMN tenant_id;
ALTER TABLE core.tasks DROP COLUMN tenant_id;
ALTER TABLE core.contact_identities DROP COLUMN tenant_id;
ALTER TABLE core.contacts DROP COLUMN tenant_id;
ALTER TABLE core.tenants DROP CONSTRAINT tenants_name_key;

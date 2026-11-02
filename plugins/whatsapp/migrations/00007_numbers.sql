-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE plugin_whatsapp.credentials DROP CONSTRAINT credentials_phone_number_id_key;
ALTER TABLE plugin_whatsapp.credentials DROP CONSTRAINT credentials_pkey;
ALTER TABLE plugin_whatsapp.credentials ADD CONSTRAINT credentials_pkey PRIMARY KEY (phone_number_id);
ALTER TABLE plugin_whatsapp.credentials
    ADD COLUMN label text NOT NULL DEFAULT '',
    ADD COLUMN app_secret bytea,
    ADD COLUMN verify_token_digest bytea,
    ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX credentials_tenant_id_idx ON plugin_whatsapp.credentials (tenant_id);

ALTER TABLE plugin_whatsapp.conversations ADD COLUMN phone_number_id text;
UPDATE plugin_whatsapp.conversations conv
SET phone_number_id = cred.phone_number_id
FROM plugin_whatsapp.credentials cred
WHERE cred.tenant_id = conv.tenant_id;
ALTER TABLE plugin_whatsapp.conversations DROP CONSTRAINT conversations_tenant_external_id_key;
ALTER TABLE plugin_whatsapp.conversations
    ADD CONSTRAINT conversations_tenant_number_external_id_key
    UNIQUE NULLS NOT DISTINCT (tenant_id, phone_number_id, external_id);

-- +goose Down
WITH ranked AS (
    SELECT id, first_value(id) OVER (PARTITION BY tenant_id, external_id ORDER BY created_at, id) AS keeper
    FROM plugin_whatsapp.conversations
)
UPDATE plugin_whatsapp.messages msg
SET conversation_id = ranked.keeper
FROM ranked
WHERE msg.conversation_id = ranked.id AND ranked.id <> ranked.keeper;

WITH newest AS (
    SELECT tenant_id, external_id, max(last_activity_at) AS last_activity_at
    FROM plugin_whatsapp.conversations
    GROUP BY tenant_id, external_id
)
UPDATE plugin_whatsapp.conversations conv
SET last_activity_at = newest.last_activity_at
FROM newest
WHERE conv.tenant_id = newest.tenant_id AND conv.external_id = newest.external_id;

DELETE FROM plugin_whatsapp.conversations conv
WHERE EXISTS (
    SELECT 1 FROM plugin_whatsapp.conversations older
    WHERE older.tenant_id = conv.tenant_id
        AND older.external_id = conv.external_id
        AND (older.created_at, older.id) < (conv.created_at, conv.id)
);

ALTER TABLE plugin_whatsapp.conversations DROP CONSTRAINT conversations_tenant_number_external_id_key;
ALTER TABLE plugin_whatsapp.conversations
    ADD CONSTRAINT conversations_tenant_external_id_key UNIQUE (tenant_id, external_id);
ALTER TABLE plugin_whatsapp.conversations DROP COLUMN phone_number_id;

DELETE FROM plugin_whatsapp.credentials cred
WHERE EXISTS (
    SELECT 1 FROM plugin_whatsapp.credentials older
    WHERE older.tenant_id = cred.tenant_id
        AND (older.created_at, older.phone_number_id) < (cred.created_at, cred.phone_number_id)
);

DROP INDEX plugin_whatsapp.credentials_tenant_id_idx;
ALTER TABLE plugin_whatsapp.credentials
    DROP COLUMN created_at,
    DROP COLUMN verify_token_digest,
    DROP COLUMN app_secret,
    DROP COLUMN label;
ALTER TABLE plugin_whatsapp.credentials DROP CONSTRAINT credentials_pkey;
ALTER TABLE plugin_whatsapp.credentials ADD CONSTRAINT credentials_pkey PRIMARY KEY (tenant_id);
ALTER TABLE plugin_whatsapp.credentials
    ADD CONSTRAINT credentials_phone_number_id_key UNIQUE (phone_number_id);

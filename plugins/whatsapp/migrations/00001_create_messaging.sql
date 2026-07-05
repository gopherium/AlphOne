-- SPDX-License-Identifier: AGPL-3.0-or-later

-- +goose Up
CREATE TABLE plugin_whatsapp.conversations (
    id uuid PRIMARY KEY,
    contact_id uuid NOT NULL REFERENCES core.contacts (id) ON DELETE CASCADE,
    channel text NOT NULL,
    external_id text NOT NULL UNIQUE,
    status text NOT NULL,
    last_activity_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE INDEX conversations_contact_id_idx ON plugin_whatsapp.conversations (contact_id);

CREATE TABLE plugin_whatsapp.messages (
    id uuid PRIMARY KEY,
    conversation_id uuid NOT NULL REFERENCES plugin_whatsapp.conversations (id) ON DELETE CASCADE,
    external_id text NOT NULL UNIQUE,
    direction text NOT NULL,
    content text NOT NULL,
    content_type text NOT NULL,
    sent_at timestamptz NOT NULL,
    raw jsonb NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE INDEX messages_conversation_id_idx ON plugin_whatsapp.messages (conversation_id);

-- +goose Down
DROP TABLE plugin_whatsapp.messages;
DROP TABLE plugin_whatsapp.conversations;

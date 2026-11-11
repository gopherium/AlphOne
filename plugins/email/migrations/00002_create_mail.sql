-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE plugin_email.threads (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    mailbox_id uuid NOT NULL REFERENCES plugin_email.mailboxes (id),
    subject text NOT NULL,
    subject_key bytea,
    first_message_at timestamptz NOT NULL,
    last_message_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX threads_subject_key_idx
    ON plugin_email.threads (tenant_id, mailbox_id, subject_key, last_message_at DESC)
    WHERE subject_key IS NOT NULL;

CREATE TABLE plugin_email.messages (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    mailbox_id uuid NOT NULL REFERENCES plugin_email.mailboxes (id),
    thread_id uuid NOT NULL REFERENCES plugin_email.threads (id),
    provider_thread_id text,
    dedupe_key bytea,
    internet_message_id text,
    in_reply_to_ids text[] NOT NULL,
    reference_ids text[] NOT NULL,
    direction text NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    subject text NOT NULL,
    participants jsonb NOT NULL,
    sent_at timestamptz,
    received_at timestamptz NOT NULL,
    body_text text NOT NULL,
    safe_html text NOT NULL,
    snippet text NOT NULL,
    body_state text NOT NULL CHECK (body_state IN ('whole', 'cut', 'headers_only')),
    skipped_parts integer NOT NULL,
    automation text NOT NULL CHECK (
        automation IN ('', 'auto-generated', 'auto-reply', 'list', 'bounce', 'abuse-report', 'read-receipt')
    ),
    encrypted boolean NOT NULL,
    signed boolean NOT NULL,
    winmail boolean NOT NULL,
    invite_method text NOT NULL CHECK (
        invite_method IN ('', 'PUBLISH', 'REQUEST', 'REPLY', 'ADD', 'CANCEL', 'REFRESH', 'COUNTER', 'DECLINECOUNTER')
    ),
    raw_file_id uuid,
    raw_size bigint NOT NULL,
    raw_sha256 bytea,
    raw_state text NOT NULL CHECK (raw_state IN ('stored', 'too_large', 'over_limit', 'none')),
    stored_bytes bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, mailbox_id, dedupe_key)
);
CREATE INDEX messages_thread_received_idx ON plugin_email.messages (thread_id, received_at, id);
CREATE INDEX messages_raw_file_id_idx ON plugin_email.messages (raw_file_id, tenant_id) WHERE raw_file_id IS NOT NULL;

CREATE TABLE plugin_email.provider_refs (
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    mailbox_id uuid NOT NULL REFERENCES plugin_email.mailboxes (id),
    provider_ref text NOT NULL,
    message_id uuid NOT NULL REFERENCES plugin_email.messages (id) ON DELETE CASCADE,
    sent_copy boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, mailbox_id, provider_ref)
);
CREATE INDEX provider_refs_message_id_idx ON plugin_email.provider_refs (message_id);

CREATE TABLE plugin_email.thread_keys (
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    mailbox_id uuid NOT NULL REFERENCES plugin_email.mailboxes (id),
    kind text NOT NULL CHECK (kind IN ('provider_thread', 'internet_message_id', 'thread_index')),
    key bytea NOT NULL,
    thread_id uuid NOT NULL REFERENCES plugin_email.threads (id) ON DELETE CASCADE,
    message_id uuid REFERENCES plugin_email.messages (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, mailbox_id, kind, key, thread_id)
);
CREATE UNIQUE INDEX thread_keys_one_thread_idx
    ON plugin_email.thread_keys (tenant_id, mailbox_id, kind, key)
    WHERE kind <> 'provider_thread';
CREATE INDEX thread_keys_thread_id_idx ON plugin_email.thread_keys (thread_id);
CREATE INDEX thread_keys_message_id_idx ON plugin_email.thread_keys (message_id) WHERE message_id IS NOT NULL;

CREATE TABLE plugin_email.message_contacts (
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    message_id uuid NOT NULL REFERENCES plugin_email.messages (id) ON DELETE CASCADE,
    contact_id uuid NOT NULL REFERENCES core.contacts (id),
    PRIMARY KEY (message_id, contact_id)
);
CREATE INDEX message_contacts_contact_id_idx ON plugin_email.message_contacts (contact_id, tenant_id, message_id);

CREATE TABLE plugin_email.attachments (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    message_id uuid NOT NULL REFERENCES plugin_email.messages (id),
    position integer NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    size bigint NOT NULL,
    sha256 bytea NOT NULL,
    inline boolean NOT NULL,
    content_id text NOT NULL,
    file_id uuid,
    state text NOT NULL CHECK (state IN ('stored', 'too_large', 'over_limit')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attachments_message_id_idx ON plugin_email.attachments (message_id);
CREATE INDEX attachments_file_id_idx ON plugin_email.attachments (file_id, tenant_id) WHERE file_id IS NOT NULL;

-- +goose Down
DROP TABLE plugin_email.attachments;
DROP TABLE plugin_email.message_contacts;
DROP TABLE plugin_email.thread_keys;
DROP TABLE plugin_email.provider_refs;
DROP TABLE plugin_email.messages;
DROP TABLE plugin_email.threads;

-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE plugin_email.attachments DROP CONSTRAINT attachments_state_check;
ALTER TABLE plugin_email.attachments ADD CONSTRAINT attachments_state_check
    CHECK (state IN ('stored', 'too_large', 'over_limit', 'deleted'));
ALTER TABLE plugin_email.messages DROP CONSTRAINT messages_raw_state_check;
ALTER TABLE plugin_email.messages ADD CONSTRAINT messages_raw_state_check
    CHECK (raw_state IN ('stored', 'too_large', 'over_limit', 'none', 'deleted'));

CREATE TABLE plugin_email.deleted_dedupe_keys (
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    mailbox_id uuid NOT NULL REFERENCES plugin_email.mailboxes (id),
    dedupe_key bytea NOT NULL,
    deleted_message_id uuid NOT NULL,
    deleted_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, mailbox_id, dedupe_key)
);

CREATE INDEX messages_stored_bytes_idx ON plugin_email.messages (tenant_id, stored_bytes DESC, id DESC);

-- +goose Down
DROP INDEX plugin_email.messages_stored_bytes_idx;
DROP TABLE plugin_email.deleted_dedupe_keys;
ALTER TABLE plugin_email.messages DROP CONSTRAINT messages_raw_state_check;
ALTER TABLE plugin_email.messages ADD CONSTRAINT messages_raw_state_check
    CHECK (raw_state IN ('stored', 'too_large', 'over_limit', 'none'));
ALTER TABLE plugin_email.attachments DROP CONSTRAINT attachments_state_check;
ALTER TABLE plugin_email.attachments ADD CONSTRAINT attachments_state_check
    CHECK (state IN ('stored', 'too_large', 'over_limit'));

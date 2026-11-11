-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE plugin_email.failed_mail (
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    mailbox_id uuid NOT NULL REFERENCES plugin_email.mailboxes (id),
    provider_ref text NOT NULL,
    reason text NOT NULL CHECK (
        reason IN ('permanent_database_error', 'temporary_database_error', 'file_store_error')
    ),
    first_failed_at timestamptz NOT NULL,
    last_failed_at timestamptz NOT NULL,
    attempts integer NOT NULL,
    PRIMARY KEY (tenant_id, mailbox_id, provider_ref)
);

-- +goose Down
DROP TABLE plugin_email.failed_mail;

-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE plugin_email.pending_files (
    tenant_id uuid NOT NULL REFERENCES core.tenants (id),
    file_id uuid NOT NULL,
    state text NOT NULL CHECK (state IN ('writing', 'deleting')),
    size bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, file_id)
);
CREATE INDEX pending_files_state_created_at_idx ON plugin_email.pending_files (state, created_at);

-- +goose Down
DROP TABLE plugin_email.pending_files;

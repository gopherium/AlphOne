-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE plugin_whatsapp.media (
    message_id uuid PRIMARY KEY REFERENCES plugin_whatsapp.messages (id) ON DELETE CASCADE,
    media_id text NOT NULL,
    status text NOT NULL,
    mime_type text NOT NULL,
    sha256 text NOT NULL,
    filename text,
    voice boolean NOT NULL DEFAULT false,
    animated boolean NOT NULL DEFAULT false,
    file_size bigint,
    data bytea,
    attempts int NOT NULL DEFAULT 0,
    last_error text,
    next_attempt_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    stored_at timestamptz
);

CREATE INDEX media_pending_idx ON plugin_whatsapp.media (next_attempt_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE plugin_whatsapp.media;

-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE plugin_whatsapp.messages
    ADD COLUMN status text,
    ADD COLUMN status_detail text;

-- +goose Down
ALTER TABLE plugin_whatsapp.messages
    DROP COLUMN status,
    DROP COLUMN status_detail;

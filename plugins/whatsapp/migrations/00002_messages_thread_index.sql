-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE INDEX messages_conversation_id_sent_at_idx
    ON plugin_whatsapp.messages (conversation_id, sent_at DESC, id DESC);

DROP INDEX plugin_whatsapp.messages_conversation_id_idx;

-- +goose Down
CREATE INDEX messages_conversation_id_idx ON plugin_whatsapp.messages (conversation_id);

DROP INDEX plugin_whatsapp.messages_conversation_id_sent_at_idx;

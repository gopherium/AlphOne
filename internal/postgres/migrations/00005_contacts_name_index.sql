-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE INDEX contacts_name_id_idx ON core.contacts (name, id);

-- +goose Down
DROP INDEX core.contacts_name_id_idx;

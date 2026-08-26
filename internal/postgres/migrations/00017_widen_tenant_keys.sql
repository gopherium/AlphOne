-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE core.user_settings DROP CONSTRAINT user_settings_pkey;
ALTER TABLE core.user_settings ADD CONSTRAINT user_settings_pkey
    PRIMARY KEY (tenant_id, user_id, key);
DROP INDEX core.tasks_origin_idx;
CREATE UNIQUE INDEX tasks_origin_idx
    ON core.tasks (tenant_id, assignee_id, origin_source, origin_event_id)
    WHERE origin_event_id IS NOT NULL;

-- +goose Down
DROP INDEX core.tasks_origin_idx;
CREATE UNIQUE INDEX tasks_origin_idx
    ON core.tasks (assignee_id, origin_source, origin_event_id)
    WHERE origin_event_id IS NOT NULL;
ALTER TABLE core.user_settings DROP CONSTRAINT user_settings_pkey;
ALTER TABLE core.user_settings ADD CONSTRAINT user_settings_pkey
    PRIMARY KEY (user_id, key);

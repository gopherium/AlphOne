-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE UNIQUE INDEX tasks_origin_idx
    ON core.tasks (assignee_id, origin_source, origin_event_id)
    WHERE origin_event_id IS NOT NULL;

-- +goose Down
DROP INDEX core.tasks_origin_idx;

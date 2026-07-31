-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE core.tasks
    DROP CONSTRAINT tasks_check,
    ADD CONSTRAINT tasks_origin_check CHECK (origin_event_id IS NULL OR origin_source IS NOT NULL);

-- +goose Down
ALTER TABLE core.tasks
    DROP CONSTRAINT tasks_origin_check,
    ADD CONSTRAINT tasks_check CHECK ((origin_source IS NULL) = (origin_event_id IS NULL));

-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE core.tasks
    ADD COLUMN due_time time NULL,
    ADD CONSTRAINT tasks_due_time_check
        CHECK (due_time < '24:00'::time AND EXTRACT(SECOND FROM due_time) = 0);
DROP INDEX IF EXISTS core.tasks_assignee_status_rank_idx;
DROP INDEX IF EXISTS core.tasks_assignee_status_day_rank_idx;
DROP INDEX IF EXISTS core.tasks_contact_status_rank_idx;
CREATE INDEX tasks_assignee_status_rank_time_idx
    ON core.tasks (assignee_id, status, (CASE WHEN status = 'open' THEN -priority ELSE 0 END), due_on,
        (CASE WHEN status = 'open' THEN COALESCE(due_time, '24:00'::time) ELSE '24:00'::time END), id);
CREATE INDEX tasks_assignee_status_day_rank_time_idx
    ON core.tasks (assignee_id, status, due_on, (CASE WHEN status = 'open' THEN -priority ELSE 0 END),
        (CASE WHEN status = 'open' THEN COALESCE(due_time, '24:00'::time) ELSE '24:00'::time END), id);
CREATE INDEX tasks_contact_status_rank_time_idx
    ON core.tasks (contact_id, status, (CASE WHEN status = 'open' THEN -priority ELSE 0 END), due_on,
        (CASE WHEN status = 'open' THEN COALESCE(due_time, '24:00'::time) ELSE '24:00'::time END), id);

-- +goose Down
DROP INDEX core.tasks_contact_status_rank_time_idx;
DROP INDEX core.tasks_assignee_status_day_rank_time_idx;
DROP INDEX core.tasks_assignee_status_rank_time_idx;
ALTER TABLE core.tasks DROP COLUMN due_time;
CREATE INDEX tasks_assignee_status_rank_idx
    ON core.tasks (assignee_id, status, (CASE WHEN status = 'open' THEN -priority ELSE 0 END), due_on, id);
CREATE INDEX tasks_assignee_status_day_rank_idx
    ON core.tasks (assignee_id, status, due_on, (CASE WHEN status = 'open' THEN -priority ELSE 0 END), id);
CREATE INDEX tasks_contact_status_rank_idx
    ON core.tasks (contact_id, status, (CASE WHEN status = 'open' THEN -priority ELSE 0 END), due_on, id);

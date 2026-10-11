-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
DROP INDEX core.tasks_assignee_status_due_idx;
DROP INDEX core.tasks_contact_status_due_idx;
CREATE INDEX tasks_assignee_status_rank_idx
    ON core.tasks (assignee_id, status, (CASE WHEN status = 'open' THEN -priority ELSE 0 END), due_on, id);
CREATE INDEX tasks_assignee_status_day_rank_idx
    ON core.tasks (assignee_id, status, due_on, (CASE WHEN status = 'open' THEN -priority ELSE 0 END), id);
CREATE INDEX tasks_contact_status_rank_idx
    ON core.tasks (contact_id, status, (CASE WHEN status = 'open' THEN -priority ELSE 0 END), due_on, id);

-- +goose Down
DROP INDEX core.tasks_contact_status_rank_idx;
DROP INDEX core.tasks_assignee_status_day_rank_idx;
DROP INDEX core.tasks_assignee_status_rank_idx;
CREATE INDEX tasks_assignee_status_due_idx ON core.tasks (assignee_id, status, due_on, id);
CREATE INDEX tasks_contact_status_due_idx ON core.tasks (contact_id, status, due_on, id);

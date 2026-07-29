-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE core.tasks (
    id uuid PRIMARY KEY,
    assignee_id uuid NOT NULL,
    contact_id uuid REFERENCES core.contacts (id) ON DELETE SET NULL,
    title text NOT NULL,
    status text NOT NULL CHECK (status IN ('open', 'done')),
    priority smallint NOT NULL,
    due_on date NOT NULL,
    origin_source text,
    origin_event_id uuid,
    created_at timestamptz NOT NULL,
    CHECK ((origin_source IS NULL) = (origin_event_id IS NULL))
);

CREATE INDEX tasks_assignee_status_due_idx ON core.tasks (assignee_id, status, due_on, id);
CREATE INDEX tasks_contact_status_due_idx ON core.tasks (contact_id, status, due_on, id);

-- +goose Down
DROP TABLE core.tasks;

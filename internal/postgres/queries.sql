-- SPDX-License-Identifier: Elastic-2.0

-- name: CreateContact :exec
INSERT INTO core.contacts (id, name, created_at)
VALUES ($1, $2, $3);

-- name: ListContacts :many
SELECT id, name, created_at
FROM core.contacts c
WHERE (c.name, c.id) > (@after_name::text, @after_id::uuid)
    AND (@query::text = '' OR c.name ILIKE '%' || @query || '%'
        OR EXISTS (
            SELECT 1 FROM core.contact_identities i
            WHERE i.contact_id = c.id
                AND (i.display_name ILIKE '%' || @query || '%'
                    OR (@digits::text <> '' AND i.identifier LIKE '%' || @digits || '%'))))
ORDER BY c.name, c.id
LIMIT @row_limit;

-- name: GetContact :one
SELECT id, name, created_at
FROM core.contacts
WHERE id = $1;

-- name: UpdateContactName :one
UPDATE core.contacts SET name = $2 WHERE id = $1
RETURNING id, name, created_at;

-- name: ListContactIdentities :many
SELECT id, contact_id, channel, identifier, display_name, created_at
FROM core.contact_identities
WHERE contact_id = $1
ORDER BY channel, identifier;

-- name: GetIdentity :one
SELECT id, contact_id, channel, identifier, display_name, created_at
FROM core.contact_identities
WHERE channel = $1 AND identifier = $2;

-- name: CreateIdentity :execrows
INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (channel, identifier) DO NOTHING;

-- name: CreateTask :exec
INSERT INTO core.tasks (id, assignee_id, contact_id, title, status, priority, due_on,
    origin_source, origin_event_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetTask :one
SELECT id, assignee_id, contact_id, title, status, priority, due_on,
    origin_source, origin_event_id, created_at
FROM core.tasks
WHERE id = $1;

-- name: ListTasksForDay :many
SELECT id, assignee_id, contact_id, title, status, priority, due_on,
    origin_source, origin_event_id, created_at
FROM core.tasks t
WHERE t.assignee_id = @assignee_id::uuid
    AND t.due_on = @due_on::date
    AND (@status::text = 'all' OR t.status = @status::text)
    AND (t.due_on, t.id) > (@after_due_on::date, @after_id::uuid)
ORDER BY t.due_on, t.id
LIMIT @row_limit;

-- name: ListTasksDueBefore :many
SELECT id, assignee_id, contact_id, title, status, priority, due_on,
    origin_source, origin_event_id, created_at
FROM core.tasks t
WHERE t.assignee_id = @assignee_id::uuid
    AND t.due_on < @due_before::date
    AND (@status::text = 'all' OR t.status = @status::text)
    AND (t.due_on, t.id) > (@after_due_on::date, @after_id::uuid)
ORDER BY t.due_on, t.id
LIMIT @row_limit;

-- name: ListTasksForContact :many
SELECT id, assignee_id, contact_id, title, status, priority, due_on,
    origin_source, origin_event_id, created_at
FROM core.tasks t
WHERE t.contact_id = @contact_id::uuid
    AND (@status::text = 'all' OR t.status = @status::text)
    AND (t.due_on, t.id) > (@after_due_on::date, @after_id::uuid)
ORDER BY t.due_on, t.id
LIMIT @row_limit;

-- name: UpdateTask :one
UPDATE core.tasks
SET title = $2, status = $3, priority = $4, due_on = $5
WHERE id = $1
RETURNING id, assignee_id, contact_id, title, status, priority, due_on,
    origin_source, origin_event_id, created_at;

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

-- name: AddIdentity :one
WITH inserted AS (
    INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)
    VALUES (@id, @contact_id, @channel, @identifier, @display_name, @created_at)
    ON CONFLICT (channel, identifier) DO NOTHING
    RETURNING contact_id
)
SELECT contact_id AS owner_id, TRUE AS created FROM inserted
UNION ALL
SELECT contact_id, FALSE
FROM core.contact_identities
WHERE channel = @channel AND identifier = @identifier
    AND NOT EXISTS (SELECT 1 FROM inserted);

-- name: DeleteIdentity :execrows
DELETE FROM core.contact_identities
WHERE id = $1 AND contact_id = $2;

-- name: CreateTask :one
INSERT INTO core.tasks (id, assignee_id, contact_id, title, status, priority, due_on,
    origin_source, origin_event_id, created_at)
VALUES (@id, @assignee_id, @contact_id, @title, @status, @priority, @due_on,
    @origin_source, @origin_event_id, @created_at)
ON CONFLICT (assignee_id, origin_source, origin_event_id) WHERE origin_event_id IS NOT NULL
DO UPDATE SET id = core.tasks.id
RETURNING id, assignee_id, contact_id, title, status, priority, due_on,
    origin_source, origin_event_id, created_at, (xmax = 0) AS created;

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

-- name: CreateAPIToken :exec
INSERT INTO core.api_tokens (id, user_id, name, token_hash, created_at, last_used_at, scopes, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetAPITokenByHash :one
SELECT id, user_id, name, token_hash, created_at, last_used_at, scopes, expires_at
FROM core.api_tokens
WHERE token_hash = $1;

-- name: TouchAPIToken :exec
UPDATE core.api_tokens
SET last_used_at = $2
WHERE id = $1;

-- name: ListAPITokensForUser :many
SELECT id, user_id, name, token_hash, created_at, last_used_at, scopes, expires_at
FROM core.api_tokens
WHERE user_id = $1
ORDER BY created_at DESC, id DESC;

-- name: RevokeAPIToken :execrows
DELETE FROM core.api_tokens
WHERE id = $1 AND user_id = $2;

-- name: CreateWebhookSubscription :exec
INSERT INTO core.webhook_subscriptions (id, user_id, url, events, secret, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListWebhookSubscriptionsForUser :many
SELECT id, user_id, url, events, secret, created_at
FROM core.webhook_subscriptions
WHERE user_id = $1
ORDER BY created_at DESC, id DESC;

-- name: ListWebhookSubscriptionsForEvent :many
SELECT id, user_id, url, events, secret, created_at
FROM core.webhook_subscriptions
WHERE $1::text = ANY (events)
ORDER BY id;

-- name: DeleteWebhookSubscription :execrows
DELETE FROM core.webhook_subscriptions
WHERE id = $1 AND user_id = $2;

-- name: CreateWebhookDelivery :exec
INSERT INTO core.webhook_deliveries (
    id, subscription_id, event_id, event_name, payload,
    attempts, deliver_after, status, last_error, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: ClaimWebhookDeliveries :many
WITH claimed AS (
    UPDATE core.webhook_deliveries
    SET attempts = attempts + 1, deliver_after = @lease::timestamptz
    WHERE id IN (
        SELECT d.id
        FROM core.webhook_deliveries d
        WHERE d.status = 'pending' AND d.deliver_after <= @due::timestamptz
        ORDER BY d.deliver_after, d.id
        LIMIT @row_limit
        FOR UPDATE SKIP LOCKED
    )
    RETURNING id, subscription_id, event_id, event_name, payload,
        attempts, deliver_after, status, last_error, created_at
)
SELECT c.id, c.subscription_id, c.event_id, c.event_name, c.payload,
    c.attempts, c.deliver_after, c.status, c.last_error, c.created_at,
    s.url, s.secret
FROM claimed c
JOIN core.webhook_subscriptions s ON s.id = c.subscription_id;

-- name: SettleWebhookDelivery :exec
UPDATE core.webhook_deliveries
SET status = $2, deliver_after = $3, last_error = $4
WHERE id = $1;

-- name: ListContactsByIDs :many
SELECT id, name, created_at
FROM core.contacts
WHERE id = ANY(@ids::uuid[]);

-- name: TenantForUser :one
SELECT t.id, t.name
FROM core.tenants t
LEFT JOIN core.tenant_members m ON m.tenant_id = t.id AND m.user_id = @user_id::uuid
WHERE m.user_id IS NOT NULL OR t.id = @default_id::uuid
ORDER BY m.user_id IS NOT NULL DESC
LIMIT 1;

-- name: UserRole :one
SELECT role
FROM core.user_roles
WHERE user_id = @user_id::uuid;

-- name: UserRoles :many
SELECT user_id, role
FROM core.user_roles
WHERE user_id = ANY(@user_ids::uuid[]);

-- name: GrantUserRole :exec
INSERT INTO core.user_roles (user_id, role)
VALUES (@user_id::uuid, @role::text)
ON CONFLICT (user_id) DO UPDATE SET role = EXCLUDED.role;

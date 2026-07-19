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

-- name: GetIdentity :one
SELECT id, contact_id, channel, identifier, display_name, created_at
FROM core.contact_identities
WHERE channel = $1 AND identifier = $2;

-- name: CreateIdentity :execrows
INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (channel, identifier) DO NOTHING;

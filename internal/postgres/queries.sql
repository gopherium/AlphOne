-- SPDX-License-Identifier: Elastic-2.0

-- name: CreateContact :exec
INSERT INTO core.contacts (id, name, created_at)
VALUES ($1, $2, $3);

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

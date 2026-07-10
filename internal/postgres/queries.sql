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

-- name: CreateUser :exec
INSERT INTO core.users (id, email, name, password_hash, disabled, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetUserByEmail :one
SELECT id, email, name, password_hash, disabled, created_at
FROM core.users
WHERE email = $1;

-- name: CreateSession :exec
INSERT INTO core.sessions (token_hash, user_id, created_at, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetUserBySession :one
SELECT u.id, u.email, u.name, u.password_hash, u.disabled, u.created_at
FROM core.sessions s
JOIN core.users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > $2 AND NOT u.disabled;

-- name: DeleteSession :exec
DELETE FROM core.sessions
WHERE token_hash = $1;

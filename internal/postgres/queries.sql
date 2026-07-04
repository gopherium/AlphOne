-- SPDX-License-Identifier: AGPL-3.0-or-later

-- name: CreateContact :exec
INSERT INTO core.contacts (id, name, created_at)
VALUES ($1, $2, $3);

-- name: GetContact :one
SELECT id, name, created_at
FROM core.contacts
WHERE id = $1;

-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
UPDATE auth.users SET role = held.role
FROM core.user_roles held
WHERE held.user_id = auth.users.id;

UPDATE auth.users SET role = 'member' WHERE role = '';

DROP TABLE core.user_roles;

-- +goose Down
CREATE TABLE core.user_roles (
    user_id uuid PRIMARY KEY REFERENCES auth.users (id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('admin', 'member')),
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO core.user_roles (user_id, role)
SELECT id, role FROM auth.users WHERE role IN ('admin', 'member');

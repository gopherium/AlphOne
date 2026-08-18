-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE core.user_roles (
    user_id uuid PRIMARY KEY REFERENCES auth.users (id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('admin', 'member')),
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO core.user_roles (user_id, role)
SELECT id, 'admin' FROM auth.users
ON CONFLICT (user_id) DO NOTHING;

-- +goose Down
DROP TABLE core.user_roles;

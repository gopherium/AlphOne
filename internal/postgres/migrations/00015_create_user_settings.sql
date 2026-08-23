-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE core.user_settings (
    user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    key text NOT NULL,
    value text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key)
);

-- +goose Down
DROP TABLE core.user_settings;

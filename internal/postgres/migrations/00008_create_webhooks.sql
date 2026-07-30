-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TABLE core.webhook_subscriptions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    url text NOT NULL,
    events text[] NOT NULL,
    secret text NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE INDEX webhook_subscriptions_user_idx
    ON core.webhook_subscriptions (user_id, created_at DESC, id);

CREATE TABLE core.webhook_deliveries (
    id uuid PRIMARY KEY,
    subscription_id uuid NOT NULL REFERENCES core.webhook_subscriptions (id) ON DELETE CASCADE,
    event_id uuid NOT NULL,
    event_name text NOT NULL,
    -- Stored as text, never jsonb, because jsonb renormalises the bytes
    -- and the signature is computed over exactly what is sent.
    payload text NOT NULL,
    attempts integer NOT NULL,
    deliver_after timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'delivered', 'failed')),
    last_error text,
    created_at timestamptz NOT NULL
);

CREATE INDEX webhook_deliveries_due_idx
    ON core.webhook_deliveries (deliver_after, id)
    WHERE status = 'pending';

-- +goose Down
DROP TABLE core.webhook_deliveries;
DROP TABLE core.webhook_subscriptions;

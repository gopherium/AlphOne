-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
UPDATE core.api_tokens SET tenant_id = placed.tenant_id
FROM core.tenant_members placed
WHERE placed.user_id = core.api_tokens.user_id
    AND core.api_tokens.tenant_id = '00000000-0000-7000-8000-000000000001'
    AND placed.tenant_id <> core.api_tokens.tenant_id;

-- +goose Down

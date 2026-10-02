-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
UPDATE plugin_importer.imports SET state = 'ready'
WHERE state = 'committing' AND mapping = '{}'::jsonb;

-- +goose Down

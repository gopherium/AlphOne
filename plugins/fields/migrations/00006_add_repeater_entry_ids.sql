-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE plugin_fields.definitions ADD CONSTRAINT definitions_sub_fields_no_id
    CHECK (NOT jsonb_path_exists(sub_fields, '$[*] ? (@.name == "id")'));

UPDATE plugin_fields.contact_values AS held
SET values = held.values || rebuilt.lists
FROM (
    SELECT v.tenant_id, v.contact_id, jsonb_object_agg(d.name, (
        SELECT jsonb_agg(listed.entry || jsonb_build_object('id', uuidv7()) ORDER BY listed.at DESC)
        FROM jsonb_array_elements(v.values -> d.name) WITH ORDINALITY AS listed (entry, at)
    )) AS lists
    FROM plugin_fields.contact_values AS v
    JOIN plugin_fields.definitions AS d
        ON d.tenant_id = v.tenant_id
        AND d.kind = 'REPEATER'
        AND jsonb_typeof(v.values -> d.name) = 'array'
        AND jsonb_array_length(v.values -> d.name) > 0
        AND NOT jsonb_path_exists(v.values -> d.name, '$[*].id')
    GROUP BY v.tenant_id, v.contact_id
) AS rebuilt
WHERE held.tenant_id = rebuilt.tenant_id AND held.contact_id = rebuilt.contact_id;

-- +goose Down
UPDATE plugin_fields.contact_values AS held
SET values = held.values || rebuilt.lists
FROM (
    SELECT v.tenant_id, v.contact_id, jsonb_object_agg(d.name, (
        SELECT jsonb_agg(listed.entry - 'id' ORDER BY listed.at DESC)
        FROM jsonb_array_elements(v.values -> d.name) WITH ORDINALITY AS listed (entry, at)
    )) AS lists
    FROM plugin_fields.contact_values AS v
    JOIN plugin_fields.definitions AS d
        ON d.tenant_id = v.tenant_id
        AND d.kind = 'REPEATER'
        AND jsonb_typeof(v.values -> d.name) = 'array'
        AND jsonb_path_exists(v.values -> d.name, '$[*].id')
    GROUP BY v.tenant_id, v.contact_id
) AS rebuilt
WHERE held.tenant_id = rebuilt.tenant_id AND held.contact_id = rebuilt.contact_id;

ALTER TABLE plugin_fields.definitions DROP CONSTRAINT definitions_sub_fields_no_id;

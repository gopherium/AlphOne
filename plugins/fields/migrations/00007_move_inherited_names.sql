-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
CREATE TEMPORARY TABLE inherited_moves ON COMMIT DROP AS
SELECT d.tenant_id, d.name AS inherited, d.name || (
    SELECT min(n) FROM generate_series(2, (
        SELECT count(*) + 2 FROM plugin_fields.definitions AS held WHERE held.tenant_id = d.tenant_id
    )) AS n
    WHERE NOT EXISTS (
        SELECT 1 FROM plugin_fields.definitions AS held
        WHERE held.tenant_id = d.tenant_id AND held.name = d.name || n
    )
) AS moved
FROM plugin_fields.definitions AS d
WHERE d.name IN ('constructor', 'hasOwnProperty', 'isPrototypeOf', 'propertyIsEnumerable',
    'toLocaleString', 'toString', 'valueOf');

UPDATE plugin_fields.contact_values AS v
SET values = v.values
    - ARRAY(SELECT m.moved FROM inherited_moves AS m WHERE m.tenant_id = v.tenant_id)
    - ARRAY(SELECT m.inherited FROM inherited_moves AS m WHERE m.tenant_id = v.tenant_id)
    || COALESCE((
        SELECT jsonb_object_agg(m.moved, v.values -> m.inherited) FROM inherited_moves AS m
        WHERE m.tenant_id = v.tenant_id AND v.values ? m.inherited
    ), '{}'::jsonb)
WHERE v.tenant_id IN (SELECT m.tenant_id FROM inherited_moves AS m);

UPDATE plugin_fields.definitions AS d
SET name = m.moved
FROM inherited_moves AS m
WHERE d.tenant_id = m.tenant_id AND d.name = m.inherited;

-- +goose Down

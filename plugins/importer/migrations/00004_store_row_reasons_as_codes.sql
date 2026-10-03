-- SPDX-License-Identifier: Elastic-2.0

-- +goose Up
ALTER TABLE plugin_importer.import_rows ALTER COLUMN reason TYPE jsonb USING CASE
    WHEN reason IS NULL OR reason = '' THEN NULL
    WHEN reason = 'the row carries no name or no contact detail'
        THEN jsonb_build_object('code', 'row_incomplete', 'meta', '{}'::jsonb)
    WHEN reason = 'the row holds a name or a contact detail AlphOne cannot use'
        THEN jsonb_build_object('code', 'contact_details_invalid', 'meta', '{}'::jsonb)
    WHEN reason = 'the contact detail already belongs to a contact'
        THEN jsonb_build_object('code', 'identity_taken', 'meta', '{}'::jsonb)
    WHEN reason ~ '^the contact detail already belongs to .'
        THEN jsonb_build_object('code', 'identity_taken_by', 'meta', jsonb_build_object(
            'ownerName', substr(reason, length('the contact detail already belongs to ') + 1)))
    WHEN reason ~ '^the row holds [0-9]{1,9} cells, the header lists [0-9]{1,9}$'
        THEN jsonb_build_object('code', 'row_cell_count_mismatch', 'meta', jsonb_build_object(
            'cells', ((regexp_match(reason, 'holds ([0-9]+) cells'))[1])::integer,
            'columns', ((regexp_match(reason, 'lists ([0-9]+)$'))[1])::integer))
    WHEN reason ~ '^the row is malformed: (record on line [0-9]+; )?parse error on line [0-9]{1,9}([^0-9]|$)'
        THEN jsonb_build_object('code', 'row_quote_misplaced', 'meta', jsonb_build_object(
            'line', ((regexp_match(reason, 'parse error on line ([0-9]+)'))[1])::integer))
    WHEN starts_with(reason, 'the row is malformed: ')
        THEN jsonb_build_object('code', 'row_malformed', 'meta', '{}'::jsonb)
    WHEN reason ~ '^(fields: )?the value does not match the kind its definition declares: [^ ]+ expects [A-Z]+$'
        THEN jsonb_build_object('code', 'value_kind_mismatch', 'meta', jsonb_build_object(
            'field', (regexp_match(reason, 'declares: ([^ ]+) expects'))[1],
            'kind', (regexp_match(reason, 'expects ([A-Z]+)$'))[1]))
    WHEN reason ~ '^no live field holds [^ ,]+(, [^ ,]+)*$'
        THEN jsonb_build_object('code', 'field_unknown', 'meta', jsonb_build_object(
            'fields', to_jsonb(string_to_array(substr(reason, length('no live field holds ') + 1), ', '))))
    WHEN reason ~ '^(fields: )?no live definition holds that name: [^ ,]+(, [^ ,]+)*$'
        THEN jsonb_build_object('code', 'field_unknown', 'meta', jsonb_build_object(
            'fields', to_jsonb(string_to_array(split_part(reason, 'that name: ', 2), ', '))))
    WHEN reason = 'a field does not accept the value this row holds'
        THEN jsonb_build_object('code', 'field_text_refused', 'meta', '{}'::jsonb)
    ELSE jsonb_build_object('code', 'legacy_text', 'meta', jsonb_build_object('text', reason))
END;
ALTER TABLE plugin_importer.import_rows ADD CONSTRAINT import_rows_reason_shape CHECK (
    reason IS NULL OR COALESCE(
        jsonb_typeof(reason -> 'code') = 'string'
        AND reason ->> 'code' ~ '^[a-z][a-z_]*$'
        AND jsonb_typeof(reason -> 'meta') = 'object',
        false
    )
);

-- +goose Down
ALTER TABLE plugin_importer.import_rows DROP CONSTRAINT import_rows_reason_shape;
ALTER TABLE plugin_importer.import_rows ALTER COLUMN reason TYPE text USING CASE reason ->> 'code'
    WHEN 'row_incomplete' THEN 'the row carries no name or no contact detail'
    WHEN 'contact_details_invalid' THEN 'the row holds a name or a contact detail AlphOne cannot use'
    WHEN 'identity_taken' THEN 'the contact detail already belongs to a contact'
    WHEN 'identity_taken_by' THEN 'the contact detail already belongs to ' || (reason #>> '{meta,ownerName}')
    WHEN 'row_cell_count_mismatch' THEN format('the row holds %s cells, the header lists %s',
        reason #>> '{meta,cells}', reason #>> '{meta,columns}')
    WHEN 'row_quote_misplaced' THEN format('the row is malformed: parse error on line %s',
        reason #>> '{meta,line}')
    WHEN 'row_malformed' THEN 'the row is malformed: unreadable'
    WHEN 'value_kind_mismatch' THEN format('fields: the value does not match the kind its definition declares: '
        || '%s expects %s', reason #>> '{meta,field}', reason #>> '{meta,kind}')
    WHEN 'field_unknown' THEN 'no live field holds '
        || replace(btrim((reason #> '{meta,fields}')::text, '[]'), '"', '')
    WHEN 'field_text_refused' THEN 'a field does not accept the value this row holds'
    WHEN 'legacy_text' THEN reason #>> '{meta,text}'
    ELSE reason ->> 'code'
END;

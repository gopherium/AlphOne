// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, _x, formatList, reasonText } from '@alphone/frontend-sdk'

/**
 * Returns the message each row reason stands for, read fresh so the loaded catalogue answers.
 * @returns The messages, keyed by the code the server stores.
 */
function rowReasonTemplates(): Record<string, string> {
	return {
		row_cell_count_mismatch: __(
			'The row does not match the header. Cells in the row: %(cells)s. Columns in the header: %(columns)s.',
			'alphone-importer',
		),
		row_quote_misplaced: __(
			'Line %(line)s of the file has a quote mark out of place, so this row was left empty.',
			'alphone-importer',
		),
		row_malformed: __('AlphOne could not read this row, so it was left empty.', 'alphone-importer'),
		row_incomplete: __('The row has no name or no address.', 'alphone-importer'),
		contact_details_invalid: __('The row holds a name or an address AlphOne cannot use.', 'alphone-importer'),
		identity_taken: __('Another contact already holds an address in this row.', 'alphone-importer'),
		identity_taken_by: __('%(ownerName)s already holds an address in this row.', 'alphone-importer'),
		value_kind_mismatch: __(
			'The value for %(field)s does not match the kind the field declares: %(kind)s.',
			'alphone-importer',
		),
		field_unknown: __('Fields that no longer exist: %(fields)s.', 'alphone-importer'),
		field_text_refused: __('A field does not accept the value this row holds.', 'alphone-importer'),
	}
}

/**
 * Returns the label each field kind reads as, read fresh so the loaded catalogue answers.
 * @returns The labels, keyed by the kind the server names.
 */
function kindLabels(): Map<string, string> {
	return new Map<string, string>([
		['TEXT', _x('Text', 'field kind', 'alphone-importer')],
		['LONGTEXT', _x('Long text', 'field kind', 'alphone-importer')],
		['NUMBER', _x('Number', 'field kind', 'alphone-importer')],
		['BOOLEAN', _x('Yes or no', 'field kind', 'alphone-importer')],
		['DATE', _x('Date', 'field kind', 'alphone-importer')],
		['SELECT', _x('Choice', 'field kind', 'alphone-importer')],
		['REPEATER', _x('Repeater', 'field kind', 'alphone-importer')],
	])
}

/**
 * Reports whether a value is an object of named values.
 * @param value - The value a server sent.
 * @returns True for an object that is neither null nor a list.
 */
function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/**
 * Reports whether a value is a list of words.
 * @param value - The value a server sent.
 * @returns True for a list holding only strings.
 */
function isWordList(value: unknown): value is string[] {
	return Array.isArray(value) && value.every((item) => typeof item === 'string')
}

/**
 * Returns one value a reason carries as the reader sees it.
 * @param name - The name the value is stored under.
 * @param value - The value the server stored.
 * @param labelOf - The labeller naming a field the way the reader sees it.
 * @returns The value to fill in, or undefined for a value of the wrong type.
 */
function spokenValue(name: string, value: unknown, labelOf: (name: string) => string): unknown {
	switch (name) {
		case 'field':
			return typeof value === 'string' ? labelOf(value) : undefined
		case 'fields':
			return isWordList(value) ? formatList(value.map(labelOf)) : undefined
		case 'kind':
			return typeof value === 'string' ? (kindLabels().get(value) ?? value) : undefined
		default:
			return value
	}
}

/**
 * Returns the values a reason carries as the reader sees them, every value of the wrong type dropped.
 * @param meta - The values the server stored.
 * @param labelOf - The labeller naming a field the way the reader sees it.
 * @returns The values to fill the message in from.
 */
function spokenMeta(meta: Record<string, unknown>, labelOf: (name: string) => string): Record<string, unknown> {
	return Object.fromEntries(
		Object.entries(meta)
			.map(([name, value]) => [name, spokenValue(name, value, labelOf)] as const)
			.filter(([, value]) => value !== undefined),
	)
}

/**
 * Returns the sentence a staged row's reason reads as in the reader's language, empty when it carries none.
 * @param reason - The reason the detail document selects.
 * @param labelOf - The labeller naming a field the way the reader sees it.
 * @returns The sentence.
 */
export function rowReasonText(
	reason: { code: string; meta: unknown } | null,
	labelOf: (name: string) => string,
): string {
	if (reason === null) {
		return ''
	}
	const meta = isRecord(reason.meta) ? reason.meta : {}
	if (reason.code === 'legacy_text' && typeof meta.text === 'string') {
		return meta.text
	}
	return reasonText({ code: reason.code, meta: spokenMeta(meta, labelOf) }, rowReasonTemplates(), reason.code)
}

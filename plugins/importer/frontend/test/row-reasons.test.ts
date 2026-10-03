// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberFormatLocale } from '@alphone/frontend-sdk'
import { expect, test } from 'vitest'

import { fieldLabeller } from '../fieldLabels'
import { rowReasonText } from '../rowReasons'

/** labelOf names the live birth date field the way the reader sees it. */
const labelOf = fieldLabeller([{ name: 'birthDate', label: 'Birth date' }])

/**
 * Returns the sentence a stored reason reads as.
 * @param code - The code the server stored.
 * @param meta - The values the server stored beside it.
 * @returns The sentence.
 */
function said(code: string, meta: unknown = {}): string {
	return rowReasonText({ code, meta }, labelOf)
}

test('every code renders its English sentence', () => {
	expect([
		said('row_cell_count_mismatch', { cells: 1, columns: 3 }),
		said('row_quote_misplaced', { line: 4 }),
		said('row_malformed'),
		said('row_incomplete'),
		said('contact_details_invalid'),
		said('identity_taken'),
		said('identity_taken_by', { ownerName: 'Maria Perez' }),
		said('value_kind_mismatch', { field: 'birthDate', kind: 'DATE' }),
		said('field_unknown', { fields: ['birthDate'] }),
		said('field_text_refused'),
	]).toEqual([
		'The row does not match the header. Cells in the row: 1. Columns in the header: 3.',
		'Line 4 of the file has a quote mark out of place, so this row was left empty.',
		'AlphOne could not read this row, so it was left empty.',
		'The row has no name or no address.',
		'The row holds a name or an address AlphOne cannot use.',
		'Another contact already holds an address in this row.',
		'Maria Perez already holds an address in this row.',
		'The value for Birth date does not match the kind the field declares: Date.',
		'Fields that no longer exist: Birth date.',
		'A field does not accept the value this row holds.',
	])
})

test('the owner name fills in as written', () => {
	expect(said('identity_taken_by', { ownerName: 'Maria Perez 100% %(cells)s' })).toBe(
		'Maria Perez 100% %(cells)s already holds an address in this row.',
	)
})

test('a live field shows its label, and a vanished one its machine name', () => {
	expect(said('value_kind_mismatch', { field: 'birthDate', kind: 'DATE' })).toContain('The value for Birth date ')
	expect(said('value_kind_mismatch', { field: 'shoeSize', kind: 'NUMBER' })).toContain('The value for shoeSize ')
})

test('a contact field shows the label the importer gives it', () => {
	expect(said('field_unknown', { fields: ['email'] })).toBe('Fields that no longer exist: Email.')
})

test('the kind shows its label, and an unknown kind shows raw', () => {
	expect(
		['TEXT', 'LONGTEXT', 'NUMBER', 'BOOLEAN', 'DATE', 'SELECT', 'REPEATER', 'TIMESTAMP'].map((kind) =>
			said('value_kind_mismatch', { field: 'birthDate', kind }).split(': ')[1],
		),
	).toEqual(['Text.', 'Long text.', 'Number.', 'Yes or no.', 'Date.', 'Choice.', 'Repeater.', 'TIMESTAMP.'])
})

test('field_unknown lists its fields', () => {
	expect(said('field_unknown', { fields: ['birthDate', 'shoeSize', 'height'] })).toBe(
		'Fields that no longer exist: Birth date, shoeSize, height.',
	)
})

test('numbers follow the format locale', () => {
	rememberFormatLocale('es-ES-u-nu-deva')

	expect(said('row_cell_count_mismatch', { cells: 2, columns: 3 })).toBe(
		'The row does not match the header. Cells in the row: २. Columns in the header: ३.',
	)
	expect(said('row_quote_misplaced', { line: 1234 })).toContain('Line १.२३४ of the file')
})

test('legacy_text shows the stored text', () => {
	expect(said('legacy_text', { text: 'the provider will not store this row' })).toBe(
		'the provider will not store this row',
	)
	expect(said('legacy_text', { text: 7 })).toBe('legacy_text')
})

test('an unknown code, missing meta values and a non-object meta read as the code', () => {
	expect(said('row_from_the_future', { cells: 2 })).toBe('row_from_the_future')
	expect(said('identity_taken_by')).toBe('identity_taken_by')
	expect(said('row_cell_count_mismatch', { cells: 2 })).toBe('row_cell_count_mismatch')
	expect(said('identity_taken_by', null)).toBe('identity_taken_by')
	expect(said('identity_taken_by', ['Maria Perez'])).toBe('identity_taken_by')
	expect(said('identity_taken_by', 'Maria Perez')).toBe('identity_taken_by')
})

test('a value of the wrong type reads as the code', () => {
	expect(said('value_kind_mismatch', { field: 7, kind: 'DATE' })).toBe('value_kind_mismatch')
	expect(said('value_kind_mismatch', { field: 'birthDate', kind: 7 })).toBe('value_kind_mismatch')
	expect(said('field_unknown', { fields: 'birthDate' })).toBe('field_unknown')
	expect(said('field_unknown', { fields: ['birthDate', 7] })).toBe('field_unknown')
})

test('a code named constructor reads as itself', () => {
	expect(said('constructor')).toBe('constructor')
	expect(said('toString', { text: 'x' })).toBe('toString')
})

test('a field named constructor reads as its own name', () => {
	expect(said('value_kind_mismatch', { field: 'constructor', kind: 'DATE' })).toBe(
		'The value for constructor does not match the kind the field declares: Date.',
	)
	expect(said('field_unknown', { fields: ['__proto__', 'toString'] })).toBe(
		'Fields that no longer exist: __proto__, toString.',
	)
})

test('null gives an empty string', () => {
	expect(rowReasonText(null, labelOf)).toBe('')
})

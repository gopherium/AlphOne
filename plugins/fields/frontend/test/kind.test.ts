// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { kindItems, kindOf, subKindItems } from '../kind'

test('a chosen item resolves to its kind', () => {
	expect(kindOf({ value: 'DATE' })).toEqual({ value: 'DATE', label: 'Date' })
})

test('the kind menu offers the repeater last', () => {
	expect(kindItems().at(-1)).toEqual({ value: 'REPEATER', label: 'Repeater' })
})

test('a sub field may hold every kind but the repeater', () => {
	expect(subKindItems().map((item) => item.value)).toEqual([
		'TEXT',
		'LONGTEXT',
		'NUMBER',
		'BOOLEAN',
		'DATE',
		'SELECT',
	])
})

test('a cleared selection falls back to text', () => {
	expect(kindOf(null)).toEqual(kindItems()[0])
	expect(kindOf({ value: null })).toEqual(kindItems()[0])
})

test('a kind the catalogue does not offer falls back to text', () => {
	expect(kindOf({ value: 'TIMESTAMP' })).toEqual(kindItems()[0])
})

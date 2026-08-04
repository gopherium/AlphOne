// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { assignmentsOf, chosenItem, chosenValue, withAssignment } from '../ImportScreen'

const items = [
	{ value: 'not-imported', label: 'Not imported' },
	{ value: 'name', label: 'Name' },
]

test('a chosen field resolves to its item', () => {
	expect(chosenItem(items, 'name')).toEqual({ value: 'name', label: 'Name' })
})

test('a field the registry no longer offers falls back to not imported', () => {
	expect(chosenItem(items, 'nickname')).toEqual({ value: 'not-imported', label: 'Not imported' })
})

test('a cleared selection reads as not imported', () => {
	expect(chosenValue(null)).toBe('not-imported')
	expect(chosenValue({ value: null })).toBe('not-imported')
	expect(chosenValue({ value: 'name' })).toBe('name')
})

test('assigning a field records it against its column index', () => {
	expect(withAssignment({}, 0, 'name')).toEqual({ '0': 'name' })
})

test('reassigning a column replaces its field', () => {
	expect(withAssignment({ '0': 'name' }, 0, 'email')).toEqual({ '0': 'email' })
})

test('setting a column back to not imported drops it', () => {
	expect(withAssignment({ '0': 'name', '1': 'email' }, 1, 'not-imported')).toEqual({
		'0': 'name',
	})
})

test('dropping the only assignment leaves nothing', () => {
	expect(withAssignment({ '0': 'name' }, 0, 'not-imported')).toEqual({})
})

test('the wire shape carries a numeric column beside its field', () => {
	expect(assignmentsOf({ '0': 'name', '2': 'phone' })).toEqual([
		{ column: 0, field: 'name' },
		{ column: 2, field: 'phone' },
	])
})

test('an empty mapping carries no assignments', () => {
	expect(assignmentsOf({})).toEqual([])
})

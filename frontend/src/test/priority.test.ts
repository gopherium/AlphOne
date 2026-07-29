import { expect, test } from 'vitest'

import { priorityOf } from '../tasks/priority'

test('reads the priority a select item stands for', () => {
	expect(priorityOf({ value: '1' })).toBe(1)
	expect(priorityOf({ value: '0' })).toBe(0)
})

test('falls back to a normal priority without a selection', () => {
	expect(priorityOf(null)).toBe(0)
	expect(priorityOf({ value: null })).toBe(0)
})

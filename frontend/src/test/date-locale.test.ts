// SPDX-License-Identifier: AGPL-3.0-or-later

import { resetLocale } from '@gopherium/gottext/testing'
import { rememberLocale } from '@gopherium/gottext'
import { afterEach, expect, test } from 'vitest'

import { formatCreated } from '../contacts/format'
import { formatDay, formatDue } from '../tasks/format'

afterEach(() => {
	resetLocale()
})

test('shows a creation date in the locale the interface stands in', () => {
	const at = new Date(2026, 6, 6)

	rememberLocale('en-US')
	const english = formatCreated(at)
	rememberLocale('es-ES')

	expect(formatCreated(at)).not.toBe(english)
	expect(english).toBe('Jul 6, 2026')
})

test('shows a day heading in the locale the interface stands in', () => {
	rememberLocale('es-ES')

	expect(formatDay('2026-07-30')).toContain('jul')
})

test('shows a due label in the locale the interface stands in', () => {
	rememberLocale('en-US')

	expect(formatDue('2026-07-30')).toBe('Due Jul 30')
})

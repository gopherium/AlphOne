// SPDX-License-Identifier: AGPL-3.0-or-later

import { resetLocale } from '@gopherium/gottext/testing'
import { rememberLocale } from '@gopherium/gottext'
import { afterEach, expect, test } from 'vitest'

import { formatDay, formatDue, movedMessage } from '../tasks/format'

afterEach(() => {
	resetLocale()
})

test('names the weekday of a day heading in the interface language beside its date in the format locale', () => {
	rememberLocale('en-US')
	const english = formatDay('2026-07-30')
	rememberLocale('es-ES')

	expect(english).toBe('Thursday, 30/07/2026')
	expect(formatDay('2026-07-30')).toBe('jueves, 30/07/2026')
})

test('shows a due label with its date in the format locale whatever the interface language', () => {
	rememberLocale('en-US')

	expect(formatDue('2026-07-30')).toBe('Due 30/07/2026')
})

test('names the day a task moved to in the format locale', () => {
	expect(movedMessage('2026-10-01')).toBe('Task moved to 01/10/2026.')
})

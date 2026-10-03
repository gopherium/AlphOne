// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberLocale } from '@alphone/frontend-sdk'
import { afterEach, expect, test } from 'vitest'

import { formatDay, formatDayLabel, formatFileSize, formatListTime } from '../format'

afterEach(() => {
	rememberLocale('en-US')
})

test('formatFileSize scales through bytes, kilobytes, and megabytes', () => {
	expect(formatFileSize(512)).toBe('512 B')
	expect(formatFileSize(1024)).toBe('1 KB')
	expect(formatFileSize(2048)).toBe('2 KB')
	expect(formatFileSize(5 * 1024 * 1024)).toBe('5,0 MB')
})

test('formatFileSize groups the thousands and writes decimals after a comma', () => {
	expect(formatFileSize(1023 * 1024)).toBe('1.023 KB')
	expect(formatFileSize(1536 * 1024 * 1024)).toBe('1.536,0 MB')
})

test('formatDay renders the local calendar date', () => {
	expect(formatDay(new Date('2026-07-06T23:30:00Z'))).toBe('2026-07-06')
})

test('formatListTime shows the clock time for same-day activity', () => {
	const now = new Date('2026-07-06T12:00:00Z')

	expect(formatListTime(new Date('2026-07-06T09:05:00Z'), now)).toBe('09:05')
})

test('formatListTime shows an evening on a 24 hour clock', () => {
	const now = new Date('2026-07-06T22:00:00Z')

	expect(formatListTime(new Date('2026-07-06T21:05:00Z'), now)).toBe('21:05')
})

test('formatListTime labels the day for older activity', () => {
	const now = new Date('2026-07-08T12:00:00Z')

	expect(formatListTime(new Date('2026-07-06T09:05:00Z'), now)).toBe('06/07/2026')
})

test('formatDayLabel names today and yesterday', () => {
	const now = new Date('2026-07-08T12:00:00Z')

	expect(formatDayLabel(new Date('2026-07-08T01:00:00Z'), now)).toBe('Today')
	expect(formatDayLabel(new Date('2026-07-07T23:00:00Z'), now)).toBe(
		'Yesterday',
	)
	expect(formatDayLabel(new Date('2026-07-01T09:00:00Z'), now)).toBe('01/07/2026')
})

test('writes an older day in the format locale whatever language the interface stands in', () => {
	const at = new Date(2026, 6, 1, 12, 0, 0)
	const now = new Date(2026, 6, 8, 12, 0, 0)

	rememberLocale('en-US')
	const english = formatDayLabel(at, now)
	rememberLocale('es-ES')

	expect(formatDayLabel(at, now)).toBe(english)
	expect(english).toBe('01/07/2026')
})

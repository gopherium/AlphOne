// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { formatDay, formatDayLabel, formatFileSize, formatListTime, formatTime } from '../format'

test('formatFileSize scales through bytes, kilobytes, and megabytes', () => {
	expect(formatFileSize(512)).toBe('512 B')
	expect(formatFileSize(1024)).toBe('1 KB')
	expect(formatFileSize(2048)).toBe('2 KB')
	expect(formatFileSize(5 * 1024 * 1024)).toBe('5.0 MB')
})

test('formatDay renders the local calendar date', () => {
	expect(formatDay(new Date('2026-07-06T23:30:00Z'))).toBe('2026-07-06')
})

test('formatTime renders a zero-padded clock time', () => {
	expect(formatTime(new Date('2026-07-06T09:05:00Z'))).toBe('09:05')
})

test('formatListTime shows the clock time for same-day activity', () => {
	const now = new Date('2026-07-06T12:00:00Z')

	expect(formatListTime(new Date('2026-07-06T09:05:00Z'), now)).toBe('09:05')
})

test('formatListTime labels the day for older activity', () => {
	const now = new Date('2026-07-08T12:00:00Z')

	expect(formatListTime(new Date('2026-07-06T09:05:00Z'), now)).toBe(
		'Jul 6, 2026',
	)
})

test('formatDayLabel names today and yesterday', () => {
	const now = new Date('2026-07-08T12:00:00Z')

	expect(formatDayLabel(new Date('2026-07-08T01:00:00Z'), now)).toBe('Today')
	expect(formatDayLabel(new Date('2026-07-07T23:00:00Z'), now)).toBe(
		'Yesterday',
	)
	expect(formatDayLabel(new Date('2026-07-01T09:00:00Z'), now)).toBe(
		'Jul 1, 2026',
	)
})

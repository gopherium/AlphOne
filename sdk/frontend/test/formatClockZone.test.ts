// SPDX-License-Identifier: AGPL-3.0-or-later

import { afterAll, afterEach, beforeAll, expect, test, vi } from 'vitest'

import { formatTime, rememberFormatLocale } from '../index'

beforeAll(() => {
	vi.stubEnv('TZ', 'Europe/Madrid')
})

afterEach(() => {
	rememberFormatLocale(undefined)
})

afterAll(() => {
	vi.unstubAllEnvs()
})

test('keeps a bare wall clock time on the hour it names in es-ES away from UTC', () => {
	rememberFormatLocale('es-ES')

	expect(formatTime('09:30')).toBe('09:30')
	expect(formatTime('23:59')).toBe('23:59')
})

test('keeps a bare wall clock time on the hour it names in en-US away from UTC', () => {
	rememberFormatLocale('en-US')

	expect(formatTime('09:30')).toBe('09:30 AM')
})

test('still writes a moment on the reader clock away from UTC', () => {
	rememberFormatLocale('es-ES')

	expect(formatTime('2026-01-15T09:30:00Z')).toBe('10:30')
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { afterAll, beforeAll, expect, test, vi } from 'vitest'

import { listDate } from '../dataviews'

beforeAll(() => {
	vi.stubEnv('TZ', 'Europe/Madrid')
})

afterAll(() => {
	vi.unstubAllEnvs()
})

test('writes a summer moment on the reader clock', () => {
	expect(listDate('2026-10-13T15:04:00Z')).toBe('Oct 13, 2026 5:04 pm')
})

test('writes a winter moment on the reader clock', () => {
	expect(listDate('2026-01-15T15:04:00Z')).toBe('Jan 15, 2026 4:04 pm')
})

test('writes a moment past midnight on the reader clock on the next day', () => {
	expect(listDate('2026-10-13T23:30:00Z')).toBe('Oct 14, 2026 1:30 am')
})

test('writes a moment handed as a date on the reader clock too', () => {
	expect(listDate(new Date('2026-01-15T15:04:00Z'))).toBe('Jan 15, 2026 4:04 pm')
})

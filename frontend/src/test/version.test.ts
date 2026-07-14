// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { expect, test } from 'vitest'

import { fetchVersion } from '../version'

test('returns the version reported by the backend', async () => {
	server.use(http.get('/api/version', () => HttpResponse.json({ version: '1.2.3' })))

	await expect(fetchVersion()).resolves.toBe('1.2.3')
})

test('rejects when the version cannot be loaded', async () => {
	server.use(
		http.get('/api/version', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)

	await expect(fetchVersion()).rejects.toThrow('500')
})

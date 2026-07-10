// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { describe, expect, it } from 'vitest'

import { InvalidCredentialsError, fetchSession, login, logout } from '../api'

const ada = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'ada@example.com',
	name: 'Ada Lovelace',
}

describe('fetchSession', () => {
	it('returns the logged-in user', async () => {
		server.use(
			http.get('/api/auth/session', () => HttpResponse.json(ada)),
		)

		await expect(fetchSession()).resolves.toEqual(ada)
	})

	it('returns null when there is no session', async () => {
		server.use(
			http.get('/api/auth/session', () =>
				HttpResponse.json({ error: 'no session' }, { status: 401 }),
			),
		)

		await expect(fetchSession()).resolves.toBeNull()
	})

	it('rejects on server failure', async () => {
		server.use(
			http.get('/api/auth/session', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(fetchSession()).rejects.toThrow('500')
	})
})

describe('login', () => {
	it('posts the credentials and returns the user', async () => {
		let body: unknown
		server.use(
			http.post('/api/auth/login', async ({ request }) => {
				body = await request.json()
				return HttpResponse.json(ada)
			}),
		)

		await expect(login('ada@example.com', 'correct horse battery')).resolves.toEqual(ada)
		expect(body).toEqual({
			email: 'ada@example.com',
			password: 'correct horse battery',
		})
	})

	it('throws InvalidCredentialsError on rejected credentials', async () => {
		server.use(
			http.post('/api/auth/login', () =>
				HttpResponse.json({ error: 'invalid credentials' }, { status: 401 }),
			),
		)

		await expect(login('ada@example.com', 'wrong')).rejects.toBeInstanceOf(
			InvalidCredentialsError,
		)
	})

	it('rejects on server failure', async () => {
		server.use(
			http.post('/api/auth/login', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(login('ada@example.com', 'correct horse battery')).rejects.toThrow('500')
	})
})

describe('logout', () => {
	it('posts to the logout endpoint', async () => {
		let called = false
		server.use(
			http.post('/api/auth/logout', () => {
				called = true
				return new HttpResponse(null, { status: 204 })
			}),
		)

		await logout()
		expect(called).toBe(true)
	})

	it('rejects on server failure', async () => {
		server.use(
			http.post('/api/auth/logout', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(logout()).rejects.toThrow('500')
	})
})

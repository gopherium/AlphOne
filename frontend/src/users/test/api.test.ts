// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { describe, expect, it } from 'vitest'

import {
	EmailTakenError,
	createUser,
	fetchUsers,
	setUserDisabled,
} from '../api'

const ada = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'ada@example.com',
	name: 'Ada Lovelace',
	disabled: false,
	created_at: '2026-07-06T10:00:00Z',
}

describe('fetchUsers', () => {
	it('returns the parsed users', async () => {
		server.use(http.get('/api/users', () => HttpResponse.json([ada])))

		const users = await fetchUsers()

		expect(users).toHaveLength(1)
		expect(users[0].email).toBe('ada@example.com')
		expect(users[0].created_at).toBeInstanceOf(Date)
	})

	it('rejects on server failure', async () => {
		server.use(
			http.get('/api/users', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(fetchUsers()).rejects.toThrow('500')
	})
})

describe('createUser', () => {
	it('posts the credentials and returns the user', async () => {
		let body: unknown
		server.use(
			http.post('/api/users', async ({ request }) => {
				body = await request.json()
				return HttpResponse.json(ada, { status: 201 })
			}),
		)

		const user = await createUser({
			email: 'ada@example.com',
			name: 'Ada Lovelace',
			password: 'correct horse battery',
		})

		expect(user.email).toBe('ada@example.com')
		expect(body).toEqual({
			email: 'ada@example.com',
			name: 'Ada Lovelace',
			password: 'correct horse battery',
		})
	})

	it('throws EmailTakenError on a duplicate email', async () => {
		server.use(
			http.post('/api/users', () =>
				HttpResponse.json({ error: 'email already taken' }, { status: 409 }),
			),
		)

		await expect(
			createUser({ email: 'ada@example.com', name: 'Ada', password: 'correct horse battery' }),
		).rejects.toBeInstanceOf(EmailTakenError)
	})

	it('rejects on server failure', async () => {
		server.use(
			http.post('/api/users', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(
			createUser({ email: 'ada@example.com', name: 'Ada', password: 'correct horse battery' }),
		).rejects.toThrow('500')
	})
})

describe('setUserDisabled', () => {
	it('patches the disabled flag for the given user', async () => {
		let body: unknown
		let path = ''
		server.use(
			http.patch('/api/users/:id', async ({ request }) => {
				path = new URL(request.url).pathname
				body = await request.json()
				return new HttpResponse(null, { status: 204 })
			}),
		)

		await setUserDisabled(ada.id, true)

		expect(path).toBe(`/api/users/${ada.id}`)
		expect(body).toEqual({ disabled: true })
	})

	it('rejects on server failure', async () => {
		server.use(
			http.patch('/api/users/:id', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(setUserDisabled(ada.id, false)).rejects.toThrow('500')
	})
})

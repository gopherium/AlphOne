// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { InvalidCredentialsError, RateLimitedError, UnauthorizedError } from '@gopherium/react-auth'
import { EmailTakenError, ValidationError } from '@gopherium/react-auth/admin'
import { expect, test } from 'vitest'

import { graphAuthTransport } from '../auth/graphTransport'

const maria = { id: 'u1', email: 'maria@example.com', name: 'Maria Perez' }

/**
 * Serves one graph response for operations whose query contains marker.
 */
function graphResponds(marker: string, body: object) {
	server.use(
		http.post('/api/graphql', async ({ request }) => {
			const posted = (await request.json()) as { query: string }
			if (!posted.query.includes(marker)) {
				return HttpResponse.json({ errors: [{ message: `unexpected operation: ${posted.query}` }] })
			}
			return HttpResponse.json(body)
		}),
	)
}

/**
 * Builds a single error response carrying an extensions code.
 */
function graphError(code: string, message = 'rejected') {
	return { data: null, errors: [{ message, extensions: { code } }] }
}

test('fetchSession returns the identity behind me', async () => {
	graphResponds('query Me', { data: { me: maria } })

	await expect(graphAuthTransport.fetchSession()).resolves.toEqual(maria)
})

test('fetchSession returns null for anonymous callers', async () => {
	graphResponds('query Me', graphError('UNAUTHENTICATED'))

	await expect(graphAuthTransport.fetchSession()).resolves.toBeNull()
})

test('login resolves the payload identity', async () => {
	graphResponds('mutation Login', { data: { login: { me: maria } } })

	await expect(graphAuthTransport.login(maria.email, 'password1234')).resolves.toEqual(maria)
})

test('login maps the auth failure codes onto the react-auth errors', async () => {
	graphResponds('mutation Login', graphError('UNAUTHENTICATED'))
	await expect(graphAuthTransport.login(maria.email, 'wrong')).rejects.toBeInstanceOf(
		InvalidCredentialsError,
	)

	graphResponds('mutation Login', graphError('RATE_LIMITED'))
	await expect(graphAuthTransport.login(maria.email, 'wrong')).rejects.toBeInstanceOf(
		RateLimitedError,
	)
})

test('logout resolves on success and throws on errors', async () => {
	graphResponds('mutation Logout', { data: { logout: true } })
	await expect(graphAuthTransport.logout()).resolves.toBeUndefined()

	graphResponds('mutation Logout', graphError('INTERNAL', 'internal error'))
	await expect(graphAuthTransport.logout()).rejects.toThrow('internal error')
})

test('isSessionRevoked distinguishes revocation from a healthy session', async () => {
	graphResponds('query Me', graphError('UNAUTHENTICATED'))
	await expect(graphAuthTransport.isSessionRevoked()).resolves.toBe(true)

	graphResponds('query Me', { data: { me: maria } })
	await expect(graphAuthTransport.isSessionRevoked()).resolves.toBe(false)
})

test('fetchUsers maps graph accounts onto the admin shape', async () => {
	graphResponds('query Users', {
		data: {
			users: [
				{ ...maria, disabled: false, createdAt: '2026-08-05T10:30:15.123456Z' },
			],
		},
	})

	const users = await graphAuthTransport.fetchUsers()

	expect(users).toHaveLength(1)
	expect(users[0]).toMatchObject({ id: maria.id, email: maria.email, disabled: false })
	expect(users[0]?.created_at).toBeInstanceOf(Date)
})

test('fetchUsers rejects anonymous callers with UnauthorizedError', async () => {
	graphResponds('query Users', graphError('UNAUTHENTICATED'))

	await expect(graphAuthTransport.fetchUsers()).rejects.toBeInstanceOf(UnauthorizedError)
})

test('createUser maps conflicts and validation onto the admin errors', async () => {
	graphResponds('mutation CreateUser', graphError('CONFLICT'))
	await expect(
		graphAuthTransport.createUser({ email: maria.email, name: maria.name, password: 'password1234' }),
	).rejects.toBeInstanceOf(EmailTakenError)

	graphResponds('mutation CreateUser', graphError('VALIDATION', 'password too short'))
	await expect(
		graphAuthTransport.createUser({ email: maria.email, name: maria.name, password: 'x' }),
	).rejects.toThrow('password too short')
})

test('setUserDisabled maps validation onto ValidationError', async () => {
	graphResponds('mutation SetUserDisabled', { data: { setUserDisabled: true } })
	await expect(graphAuthTransport.setUserDisabled('u2', true)).resolves.toBeUndefined()

	graphResponds('mutation SetUserDisabled', graphError('VALIDATION', 'cannot disable your own account'))
	await expect(graphAuthTransport.setUserDisabled('u1', true)).rejects.toBeInstanceOf(
		ValidationError,
	)
})

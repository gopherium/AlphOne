// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { InvalidCredentialsError, RateLimitedError, UnauthorizedError } from '@gopherium/react-auth'
import { EmailTakenError, ValidationError } from '@gopherium/react-auth/admin'
import { print } from 'graphql'
import { expect, test } from 'vitest'

import { graphAuthTransport } from '../auth/graphTransport'
import { loginMutation, meQuery } from '../auth/operations'

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

test('fetchSession maps rate limiting and transport failures', async () => {
	server.use(http.post('/api/graphql', () => HttpResponse.json({}, { status: 429 })))
	await expect(graphAuthTransport.fetchSession()).rejects.toBeInstanceOf(RateLimitedError)

	server.use(http.post('/api/graphql', () => HttpResponse.json({}, { status: 500 })))
	await expect(graphAuthTransport.fetchSession()).rejects.toThrow(
		'graphql request failed with status 500',
	)
})

test('fetchSession throws the fallback message without a payload or errors', async () => {
	graphResponds('query Me', { data: null })

	await expect(graphAuthTransport.fetchSession()).rejects.toThrow('loading session failed')
})

test('login resolves the payload identity', async () => {
	graphResponds('mutation Login', { data: { login: { me: maria } } })

	await expect(graphAuthTransport.login(maria.email, 'password1234')).resolves.toEqual(maria)
})

test('every operation seeding the session asks for the tier', () => {
	for (const document of [meQuery, loginMutation]) {
		expect(print(document)).toContain('role')
	}
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

test('login throws the graph failure without a payload', async () => {
	graphResponds('mutation Login', graphError('INTERNAL', 'login failed upstream'))

	await expect(graphAuthTransport.login(maria.email, 'password1234')).rejects.toThrow(
		'login failed upstream',
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

test('fetchUsers throws the graph failure without a payload', async () => {
	graphResponds('query Users', graphError('INTERNAL', 'listing users failed upstream'))

	await expect(graphAuthTransport.fetchUsers()).rejects.toThrow('listing users failed upstream')
})

test('createUser resolves the created account', async () => {
	graphResponds('mutation CreateUser', {
		data: {
			createUser: { ...maria, disabled: false, createdAt: '2026-08-05T10:30:15.123456Z' },
		},
	})

	const account = await graphAuthTransport.createUser({
		email: maria.email,
		name: maria.name,
		password: 'password1234',
	})

	expect(account).toMatchObject({ id: maria.id, email: maria.email, disabled: false })
	expect(account.created_at).toBeInstanceOf(Date)
})

test('createUser rejects anonymous callers with UnauthorizedError', async () => {
	graphResponds('mutation CreateUser', graphError('UNAUTHENTICATED'))

	await expect(
		graphAuthTransport.createUser({ email: maria.email, name: maria.name, password: 'password1234' }),
	).rejects.toBeInstanceOf(UnauthorizedError)
})

test('createUser throws the graph failure without a payload', async () => {
	graphResponds('mutation CreateUser', graphError('INTERNAL', 'creating user failed upstream'))

	await expect(
		graphAuthTransport.createUser({ email: maria.email, name: maria.name, password: 'password1234' }),
	).rejects.toThrow('creating user failed upstream')
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

test('setUserDisabled rejects anonymous callers with UnauthorizedError', async () => {
	graphResponds('mutation SetUserDisabled', graphError('UNAUTHENTICATED'))

	await expect(graphAuthTransport.setUserDisabled('u2', true)).rejects.toBeInstanceOf(
		UnauthorizedError,
	)
})

test('setUserDisabled throws the graph failure without a mapped code', async () => {
	graphResponds('mutation SetUserDisabled', graphError('INTERNAL', 'updating user failed upstream'))

	await expect(graphAuthTransport.setUserDisabled('u2', true)).rejects.toThrow(
		'updating user failed upstream',
	)
})

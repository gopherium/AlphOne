// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { InvalidCredentialsError, RateLimitedError, UnauthorizedError } from '@gopherium/react-auth'
import {
	LastPrivilegedError,
	RoleChangedError,
	RoleNotGrantableError,
	RoleRefusedError,
	RoleUnknownError,
	SelfDisableError,
	SelfRoleError,
	UserNotEditableError,
	UserNotFoundError,
	ValidationError,
} from '@gopherium/react-auth/admin'
import { print } from 'graphql'
import { expect, test } from 'vitest'

import { graphAuthTransport } from '../auth/graphTransport'
import { loginMutation, meQuery } from '../auth/operations'

const maria = { id: 'u1', email: 'maria@example.com', name: 'Maria Perez' }

const setUserRoleOperation = 'mutation SetUserRole'
const setUserDisabledOperation = 'mutation SetUserDisabled'
const resendInviteOperation = 'mutation ResendInvite'
const inviteOperation = 'mutation Invite'
const validationCode = 'VALIDATION'
const unauthorizedCode = 'UNAUTHORIZED'
const internalCode = 'INTERNAL'
const scopeMissingReason = 'scope_missing'
const scopeNeeded = { scope: 'users:write' }
const upstreamFailure = 'the write failed upstream'
const editorRole = 'editor'
const activationLink = '/activate?token=t-1'
const crossOriginReason = 'request_cross_origin'
const crossOriginWords = 'cross-origin request refused'
const deactivatedReason = 'tenant_deactivated'
const deactivatedWords = 'tenant deactivated'

/** GraphPost is one request body the graph endpoint received. */
interface GraphPost {
	query: string
	variables?: Record<string, unknown>
}

/**
 * Serves one graph response for operations whose query contains marker, recording every post.
 */
function graphResponds(marker: string, body: object): GraphPost[] {
	const received: GraphPost[] = []
	server.use(
		http.post('/api/graphql', async ({ request }) => {
			const posted = (await request.json()) as GraphPost
			received.push(posted)
			if (!posted.query.includes(marker)) {
				return HttpResponse.json({ errors: [{ message: `unexpected operation: ${posted.query}` }] })
			}
			return HttpResponse.json(body)
		}),
	)
	return received
}

/**
 * Builds a single error response carrying an extensions code.
 */
function graphError(code: string, message = 'rejected') {
	return { data: null, errors: [{ message, extensions: { code } }] }
}

/**
 * Builds a single error response naming the reason the graph refused for.
 */
function graphRefusal(code: string, reason: string, meta?: Record<string, unknown>) {
	return { data: null, errors: [{ message: 'refused', extensions: { code, reason, meta } }] }
}

/**
 * Returns what a write rejects with, or undefined when it resolves.
 */
async function refusalOf(write: Promise<unknown>): Promise<unknown> {
	return write.then(() => undefined, (error: unknown) => error)
}

/** accountWrites pairs each account write with the operation it posts. */
const accountWrites = [
	{ operation: setUserRoleOperation, write: () => graphAuthTransport.setUserRole(maria.id, editorRole) },
	{ operation: setUserDisabledOperation, write: () => graphAuthTransport.setUserDisabled(maria.id, true) },
	{ operation: resendInviteOperation, write: () => graphAuthTransport.resendInvite(maria.email) },
	{ operation: inviteOperation, write: () => graphAuthTransport.invite({ email: maria.email, name: maria.name }) },
]

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

test('setUserDisabled resolves on true and carries a refusal reason as code with its meta', async () => {
	graphResponds(setUserDisabledOperation, { data: { setUserDisabled: true } })
	await expect(graphAuthTransport.setUserDisabled('u2', true)).resolves.toBeUndefined()

	graphResponds(setUserDisabledOperation, graphRefusal(validationCode, scopeMissingReason, scopeNeeded))
	const refused = await refusalOf(graphAuthTransport.setUserDisabled('u2', true))

	expect(refused).toBeInstanceOf(ValidationError)
	expect(refused).toMatchObject({ code: scopeMissingReason, meta: scopeNeeded })
})

test('every account write refused outside validation fails as a plain error carrying the reason', async () => {
	for (const { operation, write } of accountWrites) {
		graphResponds(operation, {
			data: null,
			errors: [{ message: deactivatedWords, extensions: { code: unauthorizedCode, reason: deactivatedReason } }],
		})
		const refused = await refusalOf(write())

		expect(refused).toBeInstanceOf(Error)
		expect(refused).not.toBeInstanceOf(ValidationError)
		expect(refused).toHaveProperty('code', deactivatedReason)
		expect(String((refused as Error).message)).not.toContain(deactivatedWords)
	}
})

test('setUserDisabled rejects anonymous callers with UnauthorizedError', async () => {
	graphResponds(setUserDisabledOperation, graphError('UNAUTHENTICATED'))

	await expect(graphAuthTransport.setUserDisabled('u2', true)).rejects.toBeInstanceOf(
		UnauthorizedError,
	)
})

test('setUserDisabled throws the graph failure without a mapped code', async () => {
	graphResponds(setUserDisabledOperation, graphError('INTERNAL', 'updating user failed upstream'))

	await expect(graphAuthTransport.setUserDisabled('u2', true)).rejects.toThrow(
		'updating user failed upstream',
	)
})

test('setUserRole posts SetUserRole and resolves on true', async () => {
	const received = graphResponds(setUserRoleOperation, { data: { setUserRole: true } })

	await expect(graphAuthTransport.setUserRole(maria.id, editorRole)).resolves.toBeUndefined()
	expect(received[0]?.variables).toEqual({ id: maria.id, role: editorRole })
})

test.each([
	{ reason: 'user_not_editable', refused: UserNotEditableError, code: 'user_not_editable' },
	{ reason: 'role_not_grantable', refused: RoleNotGrantableError, code: 'role_not_grantable' },
	{ reason: 'role_unknown', refused: RoleUnknownError, code: 'role_unknown' },
	{ reason: 'self_role_refused', refused: SelfRoleError, code: 'self_role_refused' },
	{ reason: 'self_disable_refused', refused: SelfDisableError, code: 'self_disable_refused' },
	{ reason: 'last_privileged_refused', refused: LastPrivilegedError, code: 'last_privileged_refused' },
	{ reason: 'role_changed', refused: RoleChangedError, code: 'role_changed' },
	{ reason: 'user_not_found', refused: UserNotFoundError, code: 'user_not_found' },
	{ reason: 'capability_missing', refused: RoleRefusedError, code: 'role_insufficient' },
])('every account write answers $reason with its typed error and the code $code', async (row) => {
	for (const { operation, write } of accountWrites) {
		graphResponds(operation, graphRefusal(validationCode, row.reason))
		const refused = await refusalOf(write())

		expect(refused).toBeInstanceOf(row.refused)
		expect(refused).toHaveProperty('code', row.code)
	}
})

test('resendInvite answers delivered, and the link when nothing mailed it', async () => {
	const received = graphResponds(resendInviteOperation, {
		data: { resendInvite: { delivered: true, activationLink: null } },
	})
	await expect(graphAuthTransport.resendInvite(maria.email)).resolves.toEqual({ delivered: true })
	expect(received[0]?.variables).toEqual({ email: maria.email })

	graphResponds(resendInviteOperation, { data: { resendInvite: { delivered: false, activationLink } } })
	await expect(graphAuthTransport.resendInvite(maria.email)).resolves.toEqual({
		delivered: false,
		activation_link: activationLink,
	})
})

test('setUserRole and resendInvite throw the graph failure without a mapped code', async () => {
	graphResponds(setUserRoleOperation, graphError(internalCode, upstreamFailure))
	await expect(graphAuthTransport.setUserRole(maria.id, editorRole)).rejects.toThrow(upstreamFailure)

	graphResponds(resendInviteOperation, graphError(internalCode, upstreamFailure))
	await expect(graphAuthTransport.resendInvite(maria.email)).rejects.toThrow(upstreamFailure)
})

test('a 403 router refusal of an account write carries request_cross_origin as code, as the SDK reads it', async () => {
	server.use(
		http.post('/api/graphql', () =>
			HttpResponse.json({ error: crossOriginWords, code: crossOriginReason }, { status: 403 })),
	)

	const refused = await refusalOf(graphAuthTransport.setUserRole(maria.id, editorRole))

	expect(refused).toBeInstanceOf(ValidationError)
	expect(refused).toHaveProperty('code', crossOriginReason)
})

test('a 403 router refusal of a password write fails with its status, as before', async () => {
	server.use(
		http.post('/api/graphql', () =>
			HttpResponse.json({ error: crossOriginWords, code: crossOriginReason }, { status: 403 })),
	)

	await expect(graphAuthTransport.resetPassword('t-1', 'correct horse battery')).rejects.toThrow(
		'graphql request failed with status 403',
	)
	await expect(graphAuthTransport.acceptInvite('t-1', 'correct horse battery')).rejects.toThrow(
		'graphql request failed with status 403',
	)
})

test('a failed answer that is not JSON fails with its status', async () => {
	server.use(http.post('/api/graphql', () => HttpResponse.text('bad gateway', { status: 502 })))

	await expect(graphAuthTransport.setUserRole(maria.id, editorRole)).rejects.toThrow(
		'graphql request failed with status 502',
	)
})

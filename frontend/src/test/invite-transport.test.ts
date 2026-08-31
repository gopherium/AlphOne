// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { InvalidTokenError, RateLimitedError, UnauthorizedError } from '@gopherium/react-auth'
import { ValidationError } from '@gopherium/react-auth/admin'
import { expect, test } from 'vitest'

import { graphAuthTransport, resendInvite } from '../auth/graphTransport'

const maria = {
	id: 'u1',
	email: 'maria@example.com',
	name: 'Maria Perez',
	role: 'member',
	capabilities: [],
	grantable: [],
}

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
 * Builds a single error response carrying an extensions code and reason.
 */
function graphError(code: string, reason?: string, message = 'rejected') {
	return { data: null, errors: [{ message, extensions: { code, reason } }] }
}

test('invite answers the delivered report', async () => {
	graphResponds('mutation Invite', {
		data: { invite: { delivered: true, activationLink: null } },
	})

	await expect(
		graphAuthTransport.invite({ email: 'maria@example.com', name: 'Maria Perez' }),
	).resolves.toEqual({ delivered: true })
})

test('invite answers the activation link when nothing mailed it', async () => {
	graphResponds('mutation Invite', {
		data: { invite: { delivered: false, activationLink: '/activate?token=t-1' } },
	})

	await expect(
		graphAuthTransport.invite({ email: 'maria@example.com', name: 'Maria Perez' }),
	).resolves.toEqual({ delivered: false, activation_link: '/activate?token=t-1' })
})

test('invite maps the refusals it can meet', async () => {
	graphResponds('mutation Invite', graphError('UNAUTHENTICATED'))
	await expect(
		graphAuthTransport.invite({ email: 'maria@example.com', name: 'Maria Perez' }),
	).rejects.toBeInstanceOf(UnauthorizedError)

	graphResponds('mutation Invite', graphError('VALIDATION', 'name_required', 'a name is required'))
	await expect(
		graphAuthTransport.invite({ email: 'maria@example.com', name: '' }),
	).rejects.toBeInstanceOf(ValidationError)

	graphResponds('mutation Invite', graphError('RATE_LIMITED'))
	await expect(
		graphAuthTransport.invite({ email: 'maria@example.com', name: 'Maria Perez' }),
	).rejects.toBeInstanceOf(RateLimitedError)
})

test('resendInvite answers the delivery report', async () => {
	graphResponds('mutation ResendInvite', {
		data: { resendInvite: { delivered: true, activationLink: null } },
	})

	await expect(resendInvite('maria@example.com')).resolves.toEqual({ delivered: true })
})

test('resendInvite refuses a caller without a session', async () => {
	graphResponds('mutation ResendInvite', graphError('UNAUTHENTICATED'))

	await expect(resendInvite('maria@example.com')).rejects.toBeInstanceOf(UnauthorizedError)
})

test('acceptInvite answers the activated identity', async () => {
	graphResponds('mutation AcceptInvite', { data: { acceptInvite: { me: maria } } })

	await expect(graphAuthTransport.acceptInvite('t-1', 'correct horse battery')).resolves.toEqual(
		maria,
	)
})

test('acceptInvite maps a dead link onto InvalidTokenError', async () => {
	graphResponds('mutation AcceptInvite', graphError('VALIDATION', 'token_invalid'))

	await expect(
		graphAuthTransport.acceptInvite('t-1', 'correct horse battery'),
	).rejects.toBeInstanceOf(InvalidTokenError)
})

test('acceptInvite maps a refused password onto ValidationError', async () => {
	graphResponds(
		'mutation AcceptInvite',
		graphError('VALIDATION', 'password_too_short', 'that password is too short'),
	)

	const attempt = graphAuthTransport.acceptInvite('t-1', 'short')
	await expect(attempt).rejects.toBeInstanceOf(ValidationError)
	await expect(attempt).rejects.toThrow('that password is too short')
})

test('acceptInvite maps throttling', async () => {
	graphResponds('mutation AcceptInvite', graphError('RATE_LIMITED'))

	await expect(
		graphAuthTransport.acceptInvite('t-1', 'correct horse battery'),
	).rejects.toBeInstanceOf(RateLimitedError)
})

test('requestPasswordReset resolves on the neutral answer', async () => {
	graphResponds('mutation RequestPasswordReset', { data: { requestPasswordReset: true } })

	await expect(graphAuthTransport.requestPasswordReset('maria@example.com')).resolves.toBeUndefined()
})

test('requestPasswordReset maps throttling', async () => {
	graphResponds('mutation RequestPasswordReset', graphError('RATE_LIMITED'))

	await expect(
		graphAuthTransport.requestPasswordReset('maria@example.com'),
	).rejects.toBeInstanceOf(RateLimitedError)
})

test('resetPassword resolves when the link is spent', async () => {
	graphResponds('mutation ResetPassword', { data: { resetPassword: true } })

	await expect(
		graphAuthTransport.resetPassword('t-1', 'brand new password'),
	).resolves.toBeUndefined()
})

test('resetPassword maps a dead link onto InvalidTokenError', async () => {
	graphResponds('mutation ResetPassword', graphError('VALIDATION', 'token_invalid'))

	await expect(graphAuthTransport.resetPassword('t-1', 'brand new password')).rejects.toBeInstanceOf(
		InvalidTokenError,
	)
})

test('fetchUsers carries the confirmation of every account', async () => {
	graphResponds('query Users', {
		data: {
			users: [
				{
					id: 'u1',
					email: 'maria@example.com',
					name: 'Maria Perez',
					disabled: false,
					confirmed: false,
					createdAt: '2026-08-31T00:00:00Z',
					role: 'member',
				},
			],
		},
	})

	const [held] = await graphAuthTransport.fetchUsers()

	expect(held.confirmed).toBe(false)
})

test('the token operations throw their fallback without a payload', async () => {
	graphResponds('mutation AcceptInvite', { data: null })
	await expect(graphAuthTransport.acceptInvite('t-1', 'correct horse battery')).rejects.toThrow(
		'activation failed',
	)

	graphResponds('mutation RequestPasswordReset', { data: null })
	await expect(graphAuthTransport.requestPasswordReset('maria@example.com')).rejects.toThrow(
		'requesting the reset failed',
	)

	graphResponds('mutation ResetPassword', { data: null })
	await expect(graphAuthTransport.resetPassword('t-1', 'brand new password')).rejects.toThrow(
		'resetting the password failed',
	)
})

test('the invitations throw their fallback without a payload', async () => {
	graphResponds('mutation Invite', { data: null })
	await expect(
		graphAuthTransport.invite({ email: 'maria@example.com', name: 'Maria Perez' }),
	).rejects.toThrow('inviting failed')

	graphResponds('mutation ResendInvite', { data: null })
	await expect(resendInvite('maria@example.com')).rejects.toThrow('resending the invitation failed')
})

test('an undelivered invitation without a link is refused', async () => {
	graphResponds('mutation Invite', {
		data: { invite: { delivered: false, activationLink: null } },
	})

	await expect(
		graphAuthTransport.invite({ email: 'maria@example.com', name: 'Maria Perez' }),
	).rejects.toThrow('the invitation carries no activation link')
})

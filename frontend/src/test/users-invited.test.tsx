// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, memberSession, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { renderAt } from './render'

/**
 * Answers the users query with one active and one invited account.
 */
function usersRespond() {
	server.use(
		graphql.query('Users', () =>
			HttpResponse.json({
				data: {
					users: [
						{
							__typename: 'User',
							id: '0198b2f0-0000-7000-8000-000000000003',
							email: 'grace@example.com',
							name: 'Grace Hopper',
							disabled: false,
							confirmed: true,
							createdAt: '2026-07-06T10:00:00Z',
							role: 'member',
						},
						{
							__typename: 'User',
							id: '0198b2f0-0000-7000-8000-000000000002',
							email: 'maria@example.com',
							name: 'Maria Perez',
							disabled: false,
							confirmed: false,
							createdAt: '2026-07-07T10:00:00Z',
							role: 'member',
						},
					],
				},
			}),
		),
	)
}

/**
 * Returns the table row showing one address.
 */
async function rowFor(email: string) {
	const cell = await screen.findByText(email)
	const row = cell.closest('tr')
	if (row === null) {
		throw new Error(`no row for ${email}`)
	}
	return within(row)
}

beforeEach(usersRespond)

test('marks an account awaiting activation as invited', async () => {
	renderAt('/users')

	const invited = await rowFor('maria@example.com')
	expect(invited.getByText('Invited')).toBeInTheDocument()

	const active = await rowFor('grace@example.com')
	expect(active.getByText('Active')).toBeInTheDocument()
	expect(active.queryByText('Invited')).not.toBeInTheDocument()
})

test('offers a resend only on the accounts still awaiting activation', async () => {
	renderAt('/users')

	const invited = await rowFor('maria@example.com')
	expect(invited.getByRole('button', { name: 'Resend invitation to Maria Perez' })).toBeInTheDocument()

	const active = await rowFor('grace@example.com')
	expect(active.queryByRole('button', { name: /Resend invitation/ })).not.toBeInTheDocument()
})

test('resends the invitation of a pending account', async () => {
	let sentTo: string | undefined
	server.use(
		graphql.mutation('ResendInvite', ({ variables }) => {
			sentTo = variables.email as string
			return HttpResponse.json({
				data: { resendInvite: { __typename: 'InvitePayload', delivered: true, activationLink: null } },
			})
		}),
	)
	renderAt('/users')
	const invited = await rowFor('maria@example.com')

	await userEvent.click(invited.getByRole('button', { name: 'Resend invitation to Maria Perez' }))

	await waitFor(() => expect(sentTo).toBe('maria@example.com'))
	expect(await screen.findByRole('status')).toHaveTextContent('Invitation sent.')
})

test('shows the activation link when no mail server delivered the resend', async () => {
	server.use(
		graphql.mutation('ResendInvite', () =>
			HttpResponse.json({
				data: {
					resendInvite: {
						__typename: 'InvitePayload',
						delivered: false,
						activationLink: '/activate?token=t-9',
					},
				},
			}),
		),
	)
	renderAt('/users')
	const invited = await rowFor('maria@example.com')

	await userEvent.click(invited.getByRole('button', { name: 'Resend invitation to Maria Perez' }))

	expect(await screen.findByLabelText('Activation link')).toHaveValue('/activate?token=t-9')
})

test('reports a resend that failed', async () => {
	server.use(
		graphql.mutation('ResendInvite', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'the relay refused', extensions: { code: 'INTERNAL' } }],
			}),
		),
	)
	renderAt('/users')
	const invited = await rowFor('maria@example.com')

	await userEvent.click(invited.getByRole('button', { name: 'Resend invitation to Maria Perez' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('the relay refused')
})

test('offers no resend to a member who may not manage users', async () => {
	renderAt('/users', memberSession)

	const invited = await rowFor('maria@example.com')
	expect(invited.getByText('Invited')).toBeInTheDocument()
	expect(invited.queryByRole('button', { name: /Resend invitation/ })).not.toBeInTheDocument()
})

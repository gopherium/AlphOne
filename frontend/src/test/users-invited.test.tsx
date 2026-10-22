// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { configureAppErrorText } from '../i18n/errors'
import { buttonClasses, renderAt } from './render'

/**
 * Answers the users query with one active, one invited and one disabled invited account.
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
							email: 'ada@example.com',
							name: 'Ada Lovelace',
							disabled: false,
							confirmed: true,
							createdAt: '2026-07-06T10:00:00Z',
							role: 'member',
							withinReach: true,
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
							withinReach: true,
						},
						{
							__typename: 'User',
							id: '0198b2f0-0000-7000-8000-000000000004',
							email: 'ana@example.com',
							name: 'Ana Lopez',
							disabled: true,
							confirmed: false,
							createdAt: '2026-07-08T10:00:00Z',
							role: 'member',
							withinReach: true,
						},
					],
				},
			}),
		),
	)
}

/**
 * Returns the table row showing one address.
 * @param email - The address the row shows.
 * @returns The row queries.
 */
async function rowFor(email: string) {
	return within(await screen.findByRole('row', { name: new RegExp(email) }))
}

/**
 * Returns the labels of the actions one row offers.
 * @param email - The address the row shows.
 * @returns The action labels.
 */
async function actionsOf(email: string) {
	await userEvent.click((await rowFor(email)).getByRole('button', { name: 'Actions' }))
	const labels = (await screen.findAllByRole('menuitem')).map((item) => item.textContent)
	await userEvent.keyboard('{Escape}')
	await waitFor(() => expect(screen.queryByRole('menuitem')).not.toBeInTheDocument())
	return labels
}

/**
 * Opens the resend modal of the invited account.
 * @returns The modal queries.
 */
async function openResend() {
	await userEvent.click((await rowFor('maria@example.com')).getByRole('button', { name: 'Actions' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: 'Resend invitation…' }))
	return within(await screen.findByRole('dialog', { name: 'Resend invitation' }))
}

beforeAll(async () => {
	await import('../users/UsersScreen')
})

beforeEach(usersRespond)

test('marks an account awaiting activation as invited', async () => {
	renderAt('/users')

	expect((await rowFor('maria@example.com')).getByText('Invited')).toBeInTheDocument()
	const active = await rowFor('ada@example.com')
	expect(active.getByText('Active')).toBeInTheDocument()
	expect(active.queryByText('Invited')).not.toBeInTheDocument()
})

test('reads a disabled account as disabled, even one that never activated', async () => {
	renderAt('/users')

	const barred = await rowFor('ana@example.com')
	expect(barred.getByText('Disabled')).toBeInTheDocument()
	expect(barred.queryByText('Invited')).not.toBeInTheDocument()
})

test('offers a resend only on the accounts still awaiting activation', async () => {
	renderAt('/users')

	expect(await actionsOf('maria@example.com')).toContain('Resend invitation…')
	expect(await actionsOf('ada@example.com')).not.toContain('Resend invitation…')
	expect(await actionsOf('ana@example.com')).not.toContain('Resend invitation…')
})

test('lists the actions that open a modal first, with an ellipsis, and the one that acts at once last', async () => {
	renderAt('/users')

	expect(await actionsOf('maria@example.com')).toEqual(['Change role…', 'Resend invitation…', 'Disable'])
})

test('lists the primary one click action first, as a WordPress row menu does', async () => {
	renderAt('/users')

	expect(await actionsOf('ana@example.com')).toEqual(['Enable', 'Change role…'])
})

test('opens the resend at the default medium width under its title, with Cancel as an accent text button', async () => {
	renderAt('/users')
	const modal = await openResend()

	expect(screen.getByRole('dialog', { name: 'Resend invitation' })).toHaveClass('has-size-medium')
	expect(modal.getByRole('heading', { name: 'Resend invitation' })).toBeInTheDocument()
	expect(modal.getByRole('button', { name: 'Close' })).toBeInTheDocument()
	expect(
		modal.getByText('Send a new invitation to "Maria Perez" (maria@example.com)? The link sent before stops working.'),
	).toBeInTheDocument()
	expect([...modal.getByRole('button', { name: 'Cancel' }).classList]).toEqual(buttonClasses('minimal'))
	expect(modal.getByRole('button', { name: 'Resend invitation' })).toBeInTheDocument()
})

test('cuts a long account name short in the resend question', async () => {
	const invited = {
		__typename: 'User',
		id: '0198b2f0-0000-7000-8000-000000000002',
		email: 'maria@example.com',
		name: 'Maria Perez of the accounting and payroll office team',
		disabled: false,
		confirmed: false,
		createdAt: '2026-07-07T10:00:00Z',
		role: 'member',
		withinReach: true,
	}
	server.use(graphql.query('Users', () => HttpResponse.json({ data: { users: [invited] } })))
	renderAt('/users')

	const modal = await openResend()

	expect(
		modal.getByText(
			'Send a new invitation to "Maria Perez of the accounting and payroll off…" (maria@example.com)? ' +
				'The link sent before stops working.',
		),
	).toBeInTheDocument()
})

test('puts the focus on Cancel when the resend opens, as a WordPress confirm does', async () => {
	const laidOut = vi
		.spyOn(HTMLElement.prototype, 'getClientRects')
		.mockReturnValue([new DOMRect(0, 0, 10, 10)] as unknown as DOMRectList)
	onTestFinished(() => laidOut.mockRestore())
	renderAt('/users')

	const modal = await openResend()

	await waitFor(() => expect(modal.getByRole('button', { name: 'Cancel' })).toHaveFocus())
})

test('greys Cancel out while the resend runs', async () => {
	server.use(graphql.mutation('ResendInvite', () => new Promise(() => {})))
	renderAt('/users')
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	await waitFor(() => expect(modal.getByRole('button', { name: 'Cancel' })).toHaveAttribute('aria-disabled', 'true'))
})

test('resends the invitation of a pending account and names the account in a toast', async () => {
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
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	expect(await screen.findByText('Invitation sent to "Maria Perez".')).toBeInTheDocument()
	expect(sentTo).toBe('maria@example.com')
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	expect((await rowFor('maria@example.com')).queryByText(/Invitation sent/)).not.toBeInTheDocument()
})

test('shows the activation link in the modal when no mail server delivered the resend', async () => {
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
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	expect(await modal.findByLabelText('Activation link')).toHaveValue('/activate?token=t-9')
	expect(screen.queryByText(/Invitation sent/)).not.toBeInTheDocument()
	await userEvent.click(modal.getByRole('button', { name: 'Done' }))
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
})

test('closes the resend modal without sending when the reader cancels', async () => {
	let sent = 0
	server.use(
		graphql.mutation('ResendInvite', () => {
			sent += 1
			return HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })
		}),
	)
	renderAt('/users')
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Cancel' }))

	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	expect(sent).toBe(0)
})

test('shows why a resend failed inside the modal in the words the reason names', async () => {
	configureAppErrorText()
	server.use(
		graphql.mutation('ResendInvite', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'invite: the address is malformed',
						extensions: { code: 'VALIDATION', reason: 'email_invalid' },
					},
				],
			}),
		),
	)
	renderAt('/users')
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	expect(await modal.findByRole('alert')).toHaveTextContent('Write the address as a full email address.')
})

test('says plainly that a resend failed when the server names no reason', async () => {
	server.use(
		graphql.mutation('ResendInvite', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'the relay refused', extensions: { code: 'INTERNAL' } }] }),
		),
	)
	renderAt('/users')
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	expect(await modal.findByRole('alert')).toHaveTextContent('The invitation could not be sent.')
	expect(screen.queryByText('the relay refused')).not.toBeInTheDocument()
})

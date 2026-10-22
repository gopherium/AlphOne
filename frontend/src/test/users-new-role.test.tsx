// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, adminSession, graphql, server } from '@alphone/frontend-sdk/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { sessionQueryKey } from '@gopherium/react-auth'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, expect, onTestFinished, test } from 'vitest'

import { startAppLocale } from '../i18n/start'
import { startingRole } from '../users/roles'
import { renderAt } from './render'

/**
 * Answers every invitation as delivered, recording the role each one asked for.
 * @returns The roles the invitations asked for.
 */
function invitedRoles() {
	const asked: unknown[] = []
	server.use(
		graphql.mutation('Invite', ({ variables }) => {
			asked.push(variables.role)
			return HttpResponse.json({
				data: { invite: { __typename: 'InvitePayload', delivered: true, activationLink: null } },
			})
		}),
	)
	return asked
}

/**
 * Refuses every invitation with one graph error carrying the server's own words beside its reason.
 */
function invitationRefused(message: string, extensions: Record<string, unknown>) {
	server.use(graphql.mutation('Invite', () => HttpResponse.json({ data: null, errors: [{ message, extensions }] })))
}

/**
 * Fills the address and the name of the invited account.
 */
async function fillAccount() {
	await userEvent.type(await screen.findByLabelText('Email'), 'maria@example.com')
	await userEvent.type(screen.getByLabelText('Name'), 'Maria Perez')
}

beforeAll(async () => {
	await import('../users/UsersScreen')
})

test('starts a new account at the member role, among the roles the reader may give', async () => {
	renderAt('/users/new')

	const select = await screen.findByRole('combobox', { name: 'Role' })
	expect(select).toHaveTextContent('Member')
	await userEvent.click(select)
	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Administrator', 'Member'])
})

test('sends the member role when the reader keeps the role the form starts at', async () => {
	const asked = invitedRoles()
	renderAt('/users/new')
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	await waitFor(() => expect(asked).toEqual(['member']))
})

test('sends the role the reader picks with the invitation', async () => {
	const asked = invitedRoles()
	renderAt('/users/new')
	await fillAccount()

	await userEvent.click(screen.getByRole('combobox', { name: 'Role' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Administrator' }))
	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	await waitFor(() => expect(asked).toEqual(['admin']))
})

test('starts at the narrowest role the reader may give when member is not one of them', async () => {
	renderAt('/users/new', { ...adminSession, grantable: ['steward', 'admin'] })

	expect(await screen.findByRole('combobox', { name: 'Role' })).toHaveTextContent('Administrator')
})

test('starts at the member role when the reader may give no role at all', () => {
	expect(startingRole([])).toBe('member')
})

test('drops the session when the invitation is unauthorized', async () => {
	server.use(
		graphql.mutation('Invite', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt('/users/new')
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test.each([
	{
		refused: 'a role nobody declares',
		message: 'unknown role',
		extensions: { code: 'VALIDATION', reason: 'role_unknown', meta: { field: 'role' } },
		sentence: 'Nothing declares that role.',
	},
	{
		refused: 'a role holding more than the reader holds',
		message: 'that role is beyond your own',
		extensions: { code: 'VALIDATION', reason: 'role_beyond_reach' },
		sentence: 'That role holds more than your own, or nothing declares it.',
	},
])('says under the form why the server refused $refused, never in its raw words', async (refusal) => {
	invitationRefused(refusal.message, refusal.extensions)
	renderAt('/users/new')
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(refusal.sentence)
	expect(screen.queryByText(refusal.message)).not.toBeInTheDocument()
})

test('says in Spanish under the form that nothing declares the role the invitation named', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	invitationRefused('unknown role', { code: 'VALIDATION', reason: 'role_unknown', meta: { field: 'role' } })
	renderAt('/users/new')
	await userEvent.type(await screen.findByLabelText('Correo electrónico'), 'maria@example.com')
	await userEvent.type(screen.getByLabelText('Nombre'), 'Maria Perez')

	await userEvent.click(screen.getByRole('button', { name: 'Enviar invitación' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('Nada declara ese rol.')
})

test('reads the role select and the role it starts at in Spanish', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	renderAt('/users/new')

	expect(await screen.findByRole('combobox', { name: 'Rol' })).toHaveTextContent('Miembro')
})

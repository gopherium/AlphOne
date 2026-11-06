// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, adminSession, graphql, server } from '@alphone/frontend-sdk/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { sessionQueryKey } from '@gopherium/react-auth'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, expect, onTestFinished, test } from 'vitest'

import { startAppLocale } from '../i18n/start'
import { startingRole } from '../users/roles'
import { gate } from './gate'
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
		message: 'you are not allowed to give users that role',
		extensions: { code: 'VALIDATION', reason: 'role_not_grantable' },
		sentence: 'Sorry, you are not allowed to give users that role.',
	},
])('says in a toast why the server refused $refused, never in its raw words', async (answer) => {
	invitationRefused(answer.message, answer.extensions)
	renderAt('/users/new')
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	expect(
		await screen.findByText(`Could not send the invitation. ${answer.sentence}`, { selector: '.godmin-toast' }),
	).toBeInTheDocument()
	expect(screen.queryByText(answer.message)).not.toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.getByLabelText('Email')).toBeValid()
	expect(screen.getByLabelText('Name')).toBeValid()
})

test('says in a Spanish toast that nothing declares the role the invitation named', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	invitationRefused('unknown role', { code: 'VALIDATION', reason: 'role_unknown', meta: { field: 'role' } })
	renderAt('/users/new')
	await userEvent.type(await screen.findByLabelText('Correo electrónico'), 'maria@example.com')
	await userEvent.type(screen.getByLabelText('Nombre'), 'Maria Perez')

	await userEvent.click(screen.getByRole('button', { name: 'Enviar invitación' }))

	expect(await screen.findByText(/Nada declara ese rol\.$/, { selector: '.godmin-toast' })).toBeInTheDocument()
})

test.each([
	{ box: 'Email', code: 'VALIDATION', reason: 'email_invalid', words: 'The email address is not correct.' },
	{
		box: 'Email',
		code: 'CONFLICT',
		reason: 'email_taken',
		words: 'This email is already registered. Please choose another one.',
	},
	{ box: 'Name', code: 'VALIDATION', reason: 'name_required', words: 'Please provide a name.' },
	{ box: 'Name', code: 'VALIDATION', reason: 'name_too_long', words: 'This name is too long.' },
])('says under $box what the reason $reason is about, and takes it away as the reader edits', async (answer) => {
	invitationRefused('the words of the server', { code: answer.code, reason: answer.reason })
	renderAt('/users/new')
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	await screen.findByText(answer.words)
	const field = screen.getByLabelText(answer.box)
	expect(field).toHaveAccessibleDescription(answer.words)
	expect(field).toHaveFocus()
	expect(document.querySelector('.godmin-toast')).toBeNull()

	await userEvent.type(field, 'x')

	await waitFor(() => expect(screen.queryByText(answer.words)).not.toBeInTheDocument())
})

test('says under the Name box of a Spanish page what the reason is about, in the words of the brick', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	invitationRefused('name too long', { code: 'VALIDATION', reason: 'name_too_long' })
	renderAt('/users/new')
	await userEvent.type(await screen.findByLabelText('Correo electrónico'), 'maria@example.com')
	const name = screen.getByLabelText('Nombre')
	await userEvent.type(name, 'Maria Perez')

	await userEvent.click(screen.getByRole('button', { name: 'Enviar invitación' }))

	await screen.findByText('Este nombre es demasiado largo.')
	expect(name).toHaveAccessibleDescription('Este nombre es demasiado largo.')
})

test('puts the message under the box it is about when both boxes hold one value', async () => {
	invitationRefused('name too long', { code: 'VALIDATION', reason: 'name_too_long' })
	renderAt('/users/new')
	const email = await screen.findByLabelText('Email')
	const name = screen.getByLabelText('Name')
	await userEvent.type(email, 'maria@example.com')
	await userEvent.type(name, 'maria@example.com')

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	await screen.findByText('This name is too long.')
	expect(name).toHaveAccessibleDescription('This name is too long.')
	expect(name).toHaveFocus()
	expect(email).not.toHaveAccessibleDescription('This name is too long.')
	expect(email).toBeValid()
})

test('keeps the message under a name that went out without the space the box still holds', async () => {
	const sent: unknown[] = []
	server.use(
		graphql.mutation('Invite', ({ variables }) => {
			sent.push(variables.name)
			return HttpResponse.json({
				data: null,
				errors: [{ message: 'name too long', extensions: { code: 'VALIDATION', reason: 'name_too_long' } }],
			})
		}),
	)
	renderAt('/users/new')
	await userEvent.type(await screen.findByLabelText('Email'), 'maria@example.com')
	const name = screen.getByLabelText('Name')
	await userEvent.type(name, 'Maria Perez ')

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	await screen.findByText('This name is too long.')
	expect(name).toHaveAccessibleDescription('This name is too long.')
	expect(sent).toEqual(['Maria Perez'])
})

test('says in a toast what the server said about a box changed while the invitation was on its way', async () => {
	const answer = gate()
	server.use(
		graphql.mutation('Invite', async () => {
			await answer.held
			return HttpResponse.json({
				data: null,
				errors: [{ message: 'name too long', extensions: { code: 'VALIDATION', reason: 'name_too_long' } }],
			})
		}),
	)
	renderAt('/users/new')
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))
	const name = screen.getByLabelText('Name')
	await userEvent.type(name, 'x')
	answer.release()

	expect(
		await screen.findByText('Could not send the invitation. This name is too long.', { selector: '.godmin-toast' }),
	).toBeInTheDocument()
	expect(name).toBeValid()
	expect(name).toHaveValue('Maria Perezx')
})

test('says in a toast what the server said about a field the form does not have', async () => {
	invitationRefused('password too long', { code: 'VALIDATION', reason: 'password_too_long' })
	renderAt('/users/new')
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	expect(
		await screen.findByText('Could not send the invitation. This password is too long.', { selector: '.godmin-toast' }),
	).toBeInTheDocument()
})

test('reads the role select and the role it starts at in Spanish', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	renderAt('/users/new')

	expect(await screen.findByRole('combobox', { name: 'Rol' })).toHaveTextContent('Miembro')
})

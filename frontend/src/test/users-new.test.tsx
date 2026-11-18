// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, expect, onTestFinished, test } from 'vitest'

import { startAppLocale } from '../i18n/start'
import { busyClasses, renderAt } from './render'

/** emptyEmailWords is what Add User says under an empty Email, as WordPress says it. */
const emptyEmailWords = 'Please enter an email address.'

/** emptyNameWords is what Add User says under an empty Name, as WordPress says it. */
const emptyNameWords = 'Please provide a name.'

/**
 * Answers every invitation as delivered, recording each one sent.
 * @returns The invitations sent.
 */
function deliveredInvites() {
	const sent: unknown[] = []
	server.use(
		graphql.mutation('Invite', ({ variables }) => {
			sent.push(variables)
			return HttpResponse.json({
				data: { invite: { __typename: 'InvitePayload', delivered: true, activationLink: null } },
			})
		}),
	)
	return sent
}

/** Answers every invitation as undelivered, with an activation link to hand over. */
function undeliveredInvites() {
	server.use(
		graphql.mutation('Invite', () =>
			HttpResponse.json({
				data: { invite: { __typename: 'InvitePayload', delivered: false, activationLink: '/activate?token=t-7' } },
			}),
		),
	)
}

/** Answers the users query with no account, for the list Add User goes back to. */
function noAccounts() {
	server.use(graphql.query('Users', () => HttpResponse.json({ data: { users: [] } })))
}

/** Reads the app in Spanish until the test ends. */
async function readInSpanish() {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
}

/**
 * Notes the page title shown as the first toast appears.
 * @returns The titles noted, none until a toast appears.
 */
function titleAtFirstToast(): string[] {
	const noted: string[] = []
	const watch = new MutationObserver(() => {
		if (noted.length === 0 && document.querySelector('.godmin-toast') !== null) {
			noted.push(document.querySelector('.godmin-page h2')?.textContent ?? '')
		}
	})
	watch.observe(document.body, { childList: true, subtree: true })
	onTestFinished(() => watch.disconnect())
	return noted
}

/** Fills the address and the name of the invited account, whatever mark their labels carry. */
async function fillAccount() {
	await userEvent.type(await screen.findByLabelText(/^Email/), 'maria@example.com')
	await userEvent.type(screen.getByLabelText(/^Name/), 'Maria Perez')
}

beforeAll(async () => {
	await import('../users/NewUserScreen')
	await import('../users/UsersScreen')
})

beforeEach(noAccounts)

test('titles the page Add User, as the list button that opens it reads', async () => {
	renderAt('/users/new')

	expect(await screen.findByRole('heading', { level: 2, name: 'Add User' })).toBeInTheDocument()
})

test('says under the title what the page does, as WordPress does', async () => {
	renderAt('/users/new')

	const subtitle = await screen.findByText('Create a brand new user and add them to this site.')
	expect(subtitle).toHaveClass('godmin-page__subtitle')
})

test('reads the title and the subtitle in Spanish', async () => {
	await readInSpanish()
	renderAt('/users/new')

	expect(await screen.findByRole('heading', { level: 2, name: 'Añadir usuario' })).toBeInTheDocument()
	expect(screen.getByText('Crea un nuevo usuario y añádelo a este sitio.')).toHaveClass('godmin-page__subtitle')
})

test('marks Email and Name as required in their labels', async () => {
	renderAt('/users/new')

	expect(await screen.findByLabelText('Email (Required)')).toBeRequired()
	expect(screen.getByLabelText('Name (Required)')).toBeRequired()
})

test('shows no words under Email and Name before the reader leaves them or sends the form', async () => {
	renderAt('/users/new')

	expect(await screen.findByLabelText('Email (Required)')).toHaveAccessibleDescription('')
	expect(screen.getByLabelText('Name (Required)')).toHaveAccessibleDescription('')
})

test('leaves Send invitation on before anything is typed', async () => {
	renderAt('/users/new')

	expect(await screen.findByRole('button', { name: 'Send invitation' })).not.toHaveAttribute('aria-disabled', 'true')
})

test('shows the words under an empty Email and Name once the form is sent, and sends nothing', async () => {
	const sent = deliveredInvites()
	renderAt('/users/new')

	await userEvent.click(await screen.findByRole('button', { name: 'Send invitation' }))

	await waitFor(() => expect(screen.getByLabelText('Email (Required)')).toHaveAccessibleDescription(emptyEmailWords))
	expect(screen.getByLabelText('Name (Required)')).toHaveAccessibleDescription(emptyNameWords)
	expect(screen.getByLabelText('Email (Required)')).toHaveFocus()
	expect(sent).toEqual([])
})

test('shows the words under an empty Email once the reader leaves it, and none yet under Name', async () => {
	renderAt('/users/new')
	const email = await screen.findByLabelText('Email (Required)')

	await userEvent.click(email)
	await userEvent.tab()

	await waitFor(() => expect(email).toHaveAccessibleDescription(emptyEmailWords))
	expect(screen.getByLabelText('Name (Required)')).toHaveAccessibleDescription('')
})

test('takes a name of spaces only for an empty one', async () => {
	const sent = deliveredInvites()
	renderAt('/users/new')
	await userEvent.type(await screen.findByLabelText('Email (Required)'), 'maria@example.com')
	await userEvent.type(screen.getByLabelText('Name (Required)'), '   ')

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	await waitFor(() => expect(screen.getByLabelText('Name (Required)')).toHaveAccessibleDescription(emptyNameWords))
	expect(sent).toEqual([])
})

test('drops the words under Name once the reader types one, and sends it', async () => {
	const sent = deliveredInvites()
	renderAt('/users/new')
	await userEvent.type(await screen.findByLabelText('Email (Required)'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))
	const name = screen.getByLabelText('Name (Required)')
	await waitFor(() => expect(name).toHaveAccessibleDescription(emptyNameWords))

	await userEvent.type(name, 'Maria Perez')

	await waitFor(() => expect(name).toHaveAccessibleDescription(''))
	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))
	await waitFor(() => expect(sent).toEqual([{ email: 'maria@example.com', name: 'Maria Perez', role: 'member' }]))
})

test('says under an empty Email and Name in Spanish what they need', async () => {
	await readInSpanish()
	renderAt('/users/new')

	await userEvent.click(await screen.findByRole('button', { name: 'Enviar invitación' }))

	await waitFor(() =>
		expect(screen.getByLabelText('Correo electrónico (Obligatorio)')).toHaveAccessibleDescription(
			'Por favor, introduce una dirección de correo electrónico.',
		),
	)
	expect(screen.getByLabelText('Nombre (Obligatorio)')).toHaveAccessibleDescription('Por favor, facilita un nombre.')
})

test('shows Send invitation loading while the invitation is on its way', async () => {
	server.use(graphql.mutation('Invite', () => new Promise(() => {})))
	renderAt('/users/new')
	await fillAccount()
	const send = screen.getByRole('button', { name: 'Send invitation' })

	await userEvent.click(send)

	await waitFor(() => expect(send).toHaveAttribute('aria-disabled', 'true'))
	expect([...send.classList]).toEqual(expect.arrayContaining(busyClasses()))
})

test('puts the focus on the activation link once it replaces the form', async () => {
	undeliveredInvites()
	renderAt('/users/new')
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	const link = await screen.findByLabelText('Activation link')
	await waitFor(() => expect(link).toHaveFocus())
})

test('moves to the list first, then says in a toast that the invitation went out', async () => {
	deliveredInvites()
	renderAt('/users/new')
	const noted = titleAtFirstToast()
	await fillAccount()

	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))

	expect(await screen.findByText('Invitation sent.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(noted).toEqual(['Users'])
})

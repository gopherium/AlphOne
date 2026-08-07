// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { defaultUser } from '@gopherium/react-auth/testing'
import { renderAt } from './render'

beforeEach(() =>
	server.use(
		http.get('/api/users', () =>
			HttpResponse.json([
				{
					id: '0198b2f0-0000-7000-8000-000000000001',
					email: 'grace@example.com',
					name: 'Grace Hopper',
					disabled: false,
					created_at: '2026-07-06T10:00:00Z',
				},
			]),
		),
	),
)

test('ghosts the rows while the accounts arrive', async () => {
	server.use(http.get('/api/users', () => new Promise(() => {})))
	renderAt('/users')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading users…')
	expect(status.closest('.godmin-loading-rows')).not.toBeNull()
})

test('serves the users screen at /users', async () => {
	renderAt('/users')

	expect(
		await screen.findByRole('heading', { name: 'Users' }),
	).toBeInTheDocument()
	expect(
		within(await screen.findByRole('row', { name: /Grace Hopper/ })).getByText(
			'grace@example.com',
		),
	).toBeInTheDocument()
})

test('follows the page template like every other screen', async () => {
	renderAt('/users')

	const heading = await screen.findByRole('heading', { level: 1, name: 'Users' })
	expect(heading.closest('.godmin-page')).not.toBeNull()
	expect(await screen.findByRole('link', { name: 'New user' })).toBeInTheDocument()
})

test('shows an empty state when no accounts exist', async () => {
	server.use(http.get('/api/users', () => HttpResponse.json([])))

	renderAt('/users')

	expect(await screen.findByText('No users yet.')).toBeInTheDocument()
})

// Another account, because the seeded list user is the signed-in one.
const colleague = {
	id: '0198b2f0-0000-7000-8000-0000000000ff',
	email: 'ada@example.com',
	name: 'Ada Lovelace',
	created_at: '2026-07-06T10:00:00Z',
}

test('disables an account and shows the new status', async () => {
	let disabled = false
	let patched: unknown = null
	server.use(
		http.get('/api/users', () => HttpResponse.json([{ ...colleague, disabled }])),
		http.patch('/api/users/:id', async ({ request }) => {
			patched = await request.json()
			disabled = true
			return new HttpResponse(null, { status: 204 })
		}),
	)
	renderAt('/users')

	await userEvent.click(await screen.findByRole('button', { name: 'Disable Ada Lovelace' }))

	expect(await screen.findByText('Disabled')).toBeInTheDocument()
	expect(patched).toEqual({ disabled: true })
})

test('reports when an account cannot be updated', async () => {
	server.use(
		http.get('/api/users', () => HttpResponse.json([{ ...colleague, disabled: false }])),
		http.patch('/api/users/:id', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt('/users')

	await userEvent.click(await screen.findByRole('button', { name: 'Disable Ada Lovelace' }))

	expect(await screen.findByText('Update failed.')).toBeInTheDocument()
})

test('offers no disable control on the signed-in account', async () => {
	renderAt('/users')

	await screen.findByRole('row', { name: new RegExp(defaultUser.name) })
	expect(screen.queryByRole('button', { name: /^Disable / })).not.toBeInTheDocument()
})

test('navigates to the users screen from the main menu', async () => {
	renderAt('/')

	await userEvent.click(
		await screen.findByRole('link', { name: 'Users' }),
	)

	expect(
		await screen.findByRole('heading', { name: 'Users' }),
	).toBeInTheDocument()
})

test('returns to the user list after creating a user', async () => {
	server.use(
		http.post('/api/users', () =>
			HttpResponse.json(
				{
					id: '0198b2f0-0000-7000-8000-000000000002',
					email: 'ada@example.com',
					name: 'Ada Lovelace',
					disabled: false,
					created_at: '2026-07-16T10:00:00Z',
				},
				{ status: 201 },
			),
		),
	)
	renderAt('/users/new')

	await userEvent.type(
		await screen.findByLabelText('Email'),
		'ada@example.com',
	)
	await userEvent.type(screen.getByLabelText('Name'), 'Ada Lovelace')
	await userEvent.type(
		screen.getByLabelText('Password'),
		'correct horse battery',
	)
	await userEvent.click(screen.getByRole('button', { name: 'Create user' }))

	expect(
		await screen.findByRole('heading', { name: 'Users' }),
	).toBeInTheDocument()
})

/**
 * Fills the new user form and submits it.
 */
async function submitNewUser() {
	await userEvent.type(await screen.findByLabelText('Email'), 'ada@example.com')
	await userEvent.type(screen.getByLabelText('Name'), 'Ada Lovelace')
	await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Create user' }))
}

test('reports a taken email in its own words', async () => {
	server.use(
		http.post('/api/users', () =>
			HttpResponse.json({ error: 'email already in use' }, { status: 409 }),
		),
	)
	renderAt('/users/new')

	await submitNewUser()

	expect(await screen.findByText('That email is already in use.')).toBeInTheDocument()
})

test('surfaces a rejection message from the backend', async () => {
	server.use(
		http.post('/api/users', () =>
			HttpResponse.json({ error: 'password too short' }, { status: 422 }),
		),
	)
	renderAt('/users/new')

	await submitNewUser()

	expect(await screen.findByText('password too short')).toBeInTheDocument()
})

test('falls back to generic copy when creation fails otherwise', async () => {
	server.use(
		http.post('/api/users', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt('/users/new')

	await submitNewUser()

	expect(await screen.findByText('The user could not be created.')).toBeInTheDocument()
})

test('drops the session when the users request is unauthorized', async () => {
	server.use(
		http.get('/api/users', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
	)

	const client = renderAt('/users')

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('keeps the session when the users request fails for other reasons', async () => {
	server.use(
		http.get('/api/users', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)

	const client = renderAt('/users')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Users could not be loaded.',
	)
	expect(client.getQueryData(sessionQueryKey)).not.toBeNull()
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { renderAt } from './render'

test('shows the signed-in user and a logout control', async () => {
	renderAt('/')

	expect(await screen.findByText('Grace Hopper')).toBeInTheDocument()
	expect(
		screen.getByRole('button', { name: 'Log out' }),
	).toBeInTheDocument()
})

test('clears the session when logging out', async () => {
	server.use(
		http.post('/api/auth/logout', () => new HttpResponse(null, { status: 204 })),
	)
	const client = renderAt('/')

	await userEvent.click(await screen.findByRole('button', { name: 'Log out' }))

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})

test('drops all cached data when logging out', async () => {
	server.use(
		http.post('/api/auth/logout', () => new HttpResponse(null, { status: 204 })),
	)
	const client = renderAt('/')
	client.setQueryData(['contacts'], [{ id: '1', name: 'Ada Lovelace' }])

	await userEvent.click(await screen.findByRole('button', { name: 'Log out' }))

	await waitFor(() =>
		expect(client.getQueryData(['contacts'])).toBeUndefined(),
	)
	expect(client.getQueryData(sessionQueryKey)).toBeNull()
})

test('shows an error when logging out fails', async () => {
	server.use(
		http.post('/api/auth/logout', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt('/')

	await userEvent.click(await screen.findByRole('button', { name: 'Log out' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Logout failed, please try again.',
	)
	expect(screen.getByRole('button', { name: 'Log out' })).toBeInTheDocument()
})

test('omits the account section when no user is present', async () => {
	renderAt('/', null)

	expect(
		await screen.findByRole('heading', { name: 'AlphOne' }),
	).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Log out' })).toBeNull()
})

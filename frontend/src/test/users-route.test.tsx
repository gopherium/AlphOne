// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '../auth/session'
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

test('navigates to the users screen from the main menu', async () => {
	renderAt('/')

	await userEvent.click(
		await screen.findByRole('link', { name: 'Users' }),
	)

	expect(
		await screen.findByRole('heading', { name: 'Users' }),
	).toBeInTheDocument()
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

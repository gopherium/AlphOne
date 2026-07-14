// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { NewUserScreen } from '../NewUserScreen'

const grace = {
	id: '0198b2f0-0000-7000-8000-000000000002',
	email: 'grace@example.com',
	name: 'Grace Hopper',
	disabled: false,
	created_at: '2026-07-06T11:00:00Z',
}

function renderNewUser() {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	const rootRoute = createRootRoute()
	const newRoute = createRoute({
		getParentRoute: () => rootRoute,
		path: '/users/new',
		component: NewUserScreen,
	})
	const listRoute = createRoute({
		getParentRoute: () => rootRoute,
		path: '/users',
		component: function UsersStub() {
			return <h1>Users list</h1>
		},
	})
	const router = createRouter({
		routeTree: rootRoute.addChildren([newRoute, listRoute]),
		history: createMemoryHistory({ initialEntries: ['/users/new'] }),
	})
	render(
		<QueryClientProvider client={client}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	)
}

async function fillForm(email: string, name: string, password: string) {
	await userEvent.type(await screen.findByLabelText('Email'), email)
	await userEvent.type(screen.getByLabelText('Name'), name)
	await userEvent.type(screen.getByLabelText('Password'), password)
	await userEvent.click(screen.getByRole('button', { name: 'Create user' }))
}

test('shows the create form with a disabled submit until it is filled', async () => {
	renderNewUser()

	expect(await screen.findByLabelText('Email')).toBeInTheDocument()
	expect(screen.getByLabelText('Name')).toBeInTheDocument()
	expect(screen.getByLabelText('Password')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Create user' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('creates a user and returns to the list', async () => {
	let body: unknown
	server.use(
		http.post('/api/users', async ({ request }) => {
			body = await request.json()
			return HttpResponse.json(grace, { status: 201 })
		}),
	)
	renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper', 'correct horse battery')

	expect(await screen.findByRole('heading', { name: 'Users list' })).toBeInTheDocument()
	expect(body).toEqual({
		email: 'grace@example.com',
		name: 'Grace Hopper',
		password: 'correct horse battery',
	})
})

test('shows a message when the email is already taken', async () => {
	server.use(
		http.post('/api/users', () =>
			HttpResponse.json({ error: 'email already taken' }, { status: 409 }),
		),
	)
	renderNewUser()

	await fillForm('ada@example.com', 'Ada', 'correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'That email is already in use.',
	)
	expect(screen.getByRole('heading', { name: 'New user' })).toBeInTheDocument()
})

test('shows a generic error when creation fails', async () => {
	server.use(
		http.post('/api/users', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper', 'correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The user could not be created.',
	)
})

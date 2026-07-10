// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { AuthGate } from '../AuthGate'

const ada = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'ada@example.com',
	name: 'Ada Lovelace',
}

function renderGate() {
	const client = new QueryClient({
		defaultOptions: {
			queries: { retry: false },
			mutations: { retry: false },
		},
	})
	render(
		<QueryClientProvider client={client}>
			<AuthGate>
				<p>Protected area</p>
			</AuthGate>
		</QueryClientProvider>,
	)
	return client
}

test('shows a loading indicator while the session resolves', () => {
	server.use(http.get('/api/auth/session', () => HttpResponse.json(ada)))
	renderGate()

	expect(screen.getByRole('status')).toBeInTheDocument()
})

test('renders the children when a session is active', async () => {
	server.use(http.get('/api/auth/session', () => HttpResponse.json(ada)))
	renderGate()

	expect(await screen.findByText('Protected area')).toBeInTheDocument()
	expect(screen.queryByLabelText('Email')).not.toBeInTheDocument()
})

test('shows the login screen when there is no session', async () => {
	server.use(
		http.get('/api/auth/session', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
	)
	renderGate()

	expect(await screen.findByLabelText('Email')).toBeInTheDocument()
	expect(screen.queryByText('Protected area')).not.toBeInTheDocument()
})

test('shows an error when the session cannot be loaded', async () => {
	server.use(
		http.get('/api/auth/session', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderGate()

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Something went wrong.',
	)
})

test('keeps the app mounted when a background session refetch fails', async () => {
	server.use(http.get('/api/auth/session', () => HttpResponse.json(ada)))
	const client = renderGate()
	expect(await screen.findByText('Protected area')).toBeInTheDocument()

	server.use(
		http.get('/api/auth/session', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	await act(() => client.invalidateQueries())
	await act(() => new Promise((resolve) => setTimeout(resolve, 50)))

	expect(screen.getByText('Protected area')).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('reveals the children after a successful login', async () => {
	server.use(
		http.get('/api/auth/session', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
		http.post('/api/auth/login', () => HttpResponse.json(ada)),
	)
	renderGate()

	await userEvent.type(await screen.findByLabelText('Email'), 'ada@example.com')
	await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))

	expect(await screen.findByText('Protected area')).toBeInTheDocument()
})

test('ignores a stale session response that resolves after login', async () => {
	server.use(
		http.get('/api/auth/session', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
		http.post('/api/auth/login', () => HttpResponse.json(ada)),
	)
	const client = renderGate()
	await screen.findByLabelText('Email')

	let releaseStaleSession = () => {}
	const staleSession = new Promise<void>((resolve) => {
		releaseStaleSession = resolve
	})
	server.use(
		http.get('/api/auth/session', async () => {
			await staleSession
			return HttpResponse.json({ error: 'no session' }, { status: 401 })
		}),
	)
	void client.invalidateQueries()

	await userEvent.type(screen.getByLabelText('Email'), 'ada@example.com')
	await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))
	expect(await screen.findByText('Protected area')).toBeInTheDocument()

	releaseStaleSession()
	await act(() => new Promise((resolve) => setTimeout(resolve, 50)))

	expect(screen.getByText('Protected area')).toBeInTheDocument()
})

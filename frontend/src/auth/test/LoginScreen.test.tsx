// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { LoginScreen } from '../LoginScreen'

const ada = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'ada@example.com',
	name: 'Ada Lovelace',
}

function renderLogin() {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	const onLogin = vi.fn()
	render(
		<QueryClientProvider client={client}>
			<LoginScreen onLogin={onLogin} />
		</QueryClientProvider>,
	)
	return onLogin
}

async function submitCredentials(email: string, password: string) {
	await userEvent.type(screen.getByLabelText('Email'), email)
	await userEvent.type(screen.getByLabelText('Password'), password)
	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))
}

test('shows the login form', () => {
	renderLogin()

	expect(screen.getByLabelText('Email')).toBeInTheDocument()
	expect(screen.getByLabelText('Password')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Log in' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('reports the user after a successful login', async () => {
	server.use(http.post('/api/auth/login', () => HttpResponse.json(ada)))
	const onLogin = renderLogin()

	await submitCredentials('ada@example.com', 'correct horse battery')

	expect(onLogin).toHaveBeenCalledWith(ada)
})

test('shows an error on rejected credentials', async () => {
	server.use(
		http.post('/api/auth/login', () =>
			HttpResponse.json({ error: 'invalid credentials' }, { status: 401 }),
		),
	)
	const onLogin = renderLogin()

	await submitCredentials('ada@example.com', 'wrong password!')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Invalid email or password.',
	)
	expect(onLogin).not.toHaveBeenCalled()
})

test('shows a generic error when the backend fails', async () => {
	server.use(
		http.post('/api/auth/login', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	const onLogin = renderLogin()

	await submitCredentials('ada@example.com', 'correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Login failed, please try again.',
	)
	expect(onLogin).not.toHaveBeenCalled()
})

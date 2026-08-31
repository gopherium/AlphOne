// SPDX-License-Identifier: AGPL-3.0-or-later

import { server } from '@alphone/frontend-sdk/testing'
import { createAuthQueryClient } from '@gopherium/react-auth'
import { activateOk, resetOk } from '@gopherium/react-auth/testing'
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { expect, test, vi } from 'vitest'

import { LoginSlot, publicAuthScreen } from '../auth/PublicAuth'

/**
 * Renders one element under a fresh auth query client.
 */
function renderScreen(node: ReactNode) {
	render(
		<QueryClientProvider client={createAuthQueryClient({ mutations: { retry: false } })}>
			{node}
		</QueryClientProvider>,
	)
}

test('an activation link renders the set password screen', async () => {
	const held = publicAuthScreen(new URL('https://crm.example.com/activate?token=t-1'), vi.fn())

	expect(held).not.toBeNull()
	renderScreen(held)

	expect(await screen.findByLabelText('Password')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Set password' })).toBeInTheDocument()
})

test('a reset link renders the reset password screen', async () => {
	const held = publicAuthScreen(
		new URL('https://crm.example.com/reset-password?token=t-2'),
		vi.fn(),
	)

	expect(held).not.toBeNull()
	renderScreen(held)

	expect(await screen.findByLabelText('Password')).toBeInTheDocument()
})

test('every other path renders no public screen', () => {
	for (const path of ['/', '/tasks', '/users', '/users/new', '/activateX']) {
		expect(publicAuthScreen(new URL(`https://crm.example.com${path}`), vi.fn())).toBeNull()
	}
})

test('the activation link hands its token to the screen and reports the way on', async () => {
	server.use(activateOk())
	const onDone = vi.fn()
	renderScreen(publicAuthScreen(new URL('https://crm.example.com/activate?token=t-1'), onDone))

	await userEvent.type(await screen.findByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Set password' }))

	await waitFor(() => expect(onDone).toHaveBeenCalled())
})

test('the reset link reports the way on once the password is replaced', async () => {
	server.use(resetOk())
	const onDone = vi.fn()
	renderScreen(
		publicAuthScreen(new URL('https://crm.example.com/reset-password?token=t-2'), onDone),
	)

	await userEvent.type(await screen.findByLabelText('Password'), 'brand new password')
	await userEvent.click(screen.getByRole('button', { name: 'Set password' }))

	await userEvent.click(await screen.findByRole('button', { name: 'Go to login' }))
	await waitFor(() => expect(onDone).toHaveBeenCalled())
})

test('a link carrying no token still renders its screen', async () => {
	renderScreen(publicAuthScreen(new URL('https://crm.example.com/activate'), vi.fn()))

	expect(await screen.findByLabelText('Password')).toBeInTheDocument()
})

test('the login slot offers the way to a password reset and back', async () => {
	renderScreen(<LoginSlot onLogin={vi.fn()} />)

	await userEvent.click(await screen.findByRole('button', { name: 'Forgot your password?' }))

	expect(await screen.findByRole('button', { name: 'Send reset link' })).toBeInTheDocument()

	await userEvent.click(screen.getByRole('button', { name: 'Back to login' }))

	expect(await screen.findByRole('button', { name: 'Log in' })).toBeInTheDocument()
})

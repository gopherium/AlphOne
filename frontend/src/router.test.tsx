// SPDX-License-Identifier: AGPL-3.0-or-later

import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { renderAt } from './test/render'

test('serves the inbox at the root path', async () => {
	renderAt('/')

	expect(await screen.findByText('John Doe')).toBeInTheDocument()
})

test('serves the thread for a conversation URL', async () => {
	renderAt('/conversations/019f4a00-0000-7000-8000-000000000001')

	expect(
		await screen.findByText('Hi, is the order ready?'),
	).toBeInTheDocument()
})

test('navigates from a conversation in the inbox to its thread', async () => {
	const user = userEvent.setup()
	renderAt('/')

	await user.click(await screen.findByRole('link', { name: 'John Doe' }))

	expect(
		await screen.findByText('I can pick it up after 5pm.'),
	).toBeInTheDocument()
})

test('shows the AlphOne masthead as a heading on every screen', async () => {
	renderAt('/conversations/019f4a00-0000-7000-8000-000000000001')

	expect(
		await screen.findByRole('heading', { name: 'AlphOne' }),
	).toBeInTheDocument()
})

test('returns to the inbox through the masthead link', async () => {
	const user = userEvent.setup()
	renderAt('/conversations/019f4a00-0000-7000-8000-000000000001')
	await screen.findByText('Hi, is the order ready?')

	await user.click(screen.getByRole('link', { name: 'AlphOne' }))

	expect(await screen.findByText('María Pérez')).toBeInTheDocument()
})

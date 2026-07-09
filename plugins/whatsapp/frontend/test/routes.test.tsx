// SPDX-License-Identifier: AGPL-3.0-or-later

import { renderPluginAt, server } from '@alphone/frontend-sdk/testing'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '../handlers'
import { plugin } from '../index'

beforeEach(() => server.use(...handlers))

test('contributes a WhatsApp entry to the host navigation', async () => {
	renderPluginAt(plugin, '/')

	expect(
		await screen.findByRole('link', { name: 'WhatsApp' }),
	).toBeInTheDocument()
})

test('navigates from the host navigation to the inbox', async () => {
	const user = userEvent.setup()
	renderPluginAt(plugin, '/')

	await user.click(await screen.findByRole('link', { name: 'WhatsApp' }))

	expect(await screen.findByText('John Doe')).toBeInTheDocument()
})

test('serves the thread for a conversation URL', async () => {
	renderPluginAt(
		plugin,
		'/whatsapp/conversations/019f4a00-0000-7000-8000-000000000001',
	)

	expect(
		await screen.findByText('Hi, is the order ready?'),
	).toBeInTheDocument()
})

test('navigates from a conversation in the inbox to its thread', async () => {
	const user = userEvent.setup()
	renderPluginAt(plugin, '/whatsapp')

	await user.click(await screen.findByRole('link', { name: /John Doe/ }))

	const log = await screen.findByRole('log', { name: 'Messages' })
	expect(
		within(log).getByText('I can pick it up after 5pm.'),
	).toBeInTheDocument()
})

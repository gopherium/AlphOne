// SPDX-License-Identifier: AGPL-3.0-or-later

import { renderPluginAt, server } from '@alphone/frontend-sdk/testing'
import { createRootRoute, createRouter } from '@tanstack/react-router'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '../handlers'
import { plugin } from '../index'
import { routes } from '../routes'

beforeEach(() => server.use(...handlers))

test('loads the thread screen in a chunk of its own, off the first paint', () => {
	const rootRoute = createRootRoute()
	const router = createRouter({ routeTree: rootRoute.addChildren(routes(rootRoute)) })
	const { component } = router.routesById['/whatsapp/conversations/$conversationId'].options

	expect(typeof (component as { preload?: unknown }).preload).toBe('function')
})

test('loads the conversation sidebar on demand, off the first paint', () => {
	const [section] = routes(createRootRoute())
	const sidebar = section.options.staticData?.Sidebar as { preload?: unknown } | undefined

	expect(typeof sidebar?.preload).toBe('function')
})

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

test('offers a titled empty state until a conversation is chosen', async () => {
	renderPluginAt(plugin, '/whatsapp')

	expect(await screen.findByText('No conversation selected.')).toBeInTheDocument()
	expect(
		screen.getByText('Pick one from the list to read its messages.'),
	).toBeInTheDocument()
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

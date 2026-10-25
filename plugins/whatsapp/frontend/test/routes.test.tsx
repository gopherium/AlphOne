// SPDX-License-Identifier: AGPL-3.0-or-later

import { adminSession, renderPluginAt, server } from '@alphone/frontend-sdk/testing'
import { createRootRoute, createRouter } from '@tanstack/react-router'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '../handlers'
import { plugin } from '../index'
import { routes } from '../routes'

beforeEach(() => server.use(...handlers))

/**
 * Returns the sidebar the host draws.
 * @returns The region named Navigation.
 */
function sidebar(): Promise<HTMLElement> {
	return screen.findByRole('region', { name: 'Navigation' })
}

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
	renderPluginAt(plugin, '/', { session: adminSession })

	expect(
		await within(await sidebar()).findByRole('link', { name: 'WhatsApp' }),
	).toBeInTheDocument()
})

test('navigates from the host navigation to the inbox', async () => {
	const user = userEvent.setup()
	renderPluginAt(plugin, '/', { session: adminSession })

	await user.click(await within(await sidebar()).findByRole('link', { name: 'WhatsApp' }))

	expect(await screen.findByText('John Doe')).toBeInTheDocument()
})

test('drills the host sidebar into the WhatsApp sub menu, under its title and Back', async () => {
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	const shown = await sidebar()
	expect(await within(shown).findByRole('heading', { level: 1, name: 'WhatsApp' })).toBeInTheDocument()
	expect(within(shown).getByRole('link', { name: 'Back' })).toBeInTheDocument()
	expect(await within(shown).findByText('John Doe')).toBeInTheDocument()
	expect(within(screen.getByRole('main')).getByText('No conversation selected.')).toBeInTheDocument()
})

test('takes Back on the WhatsApp sub menu home, the focus on the WhatsApp row', async () => {
	const user = userEvent.setup()
	renderPluginAt(plugin, '/', { session: adminSession })
	await user.click(await within(await sidebar()).findByRole('link', { name: 'WhatsApp' }))

	await user.click(await within(await sidebar()).findByRole('link', { name: 'Back' }))

	expect(await screen.findByText('Test host home')).toBeInTheDocument()
	const row = within(await sidebar()).getByRole('link', { name: 'WhatsApp' })
	await waitFor(() => expect(row).toHaveFocus())
})

test('offers a titled empty state until a conversation is chosen', async () => {
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	expect(await screen.findByText('No conversation selected.')).toBeInTheDocument()
	expect(
		screen.getByText('Pick one from the list to read its messages.'),
	).toBeInTheDocument()
	expect(screen.getByRole('heading', { level: 3, name: 'No conversation selected.' })).toBeInTheDocument()
})

test('serves the thread for a conversation URL', async () => {
	renderPluginAt(
		plugin,
		'/whatsapp/conversations/019f4a00-0000-7000-8000-000000000001',
		{ session: adminSession },
	)

	expect(
		await screen.findByText('Hi, is the order ready?'),
	).toBeInTheDocument()
})

test('navigates from a conversation in the inbox to its thread', async () => {
	const user = userEvent.setup()
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	await user.click(await screen.findByRole('link', { name: /John Doe/ }))

	const log = await screen.findByRole('log', { name: 'Messages' })
	expect(
		within(log).getByText('I can pick it up after 5pm.'),
	).toBeInTheDocument()
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { adminSession, renderPluginAt, server } from '@alphone/frontend-sdk/testing'
import { createRootRoute, createRouter } from '@tanstack/react-router'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '../handlers'
import { plugin } from '../index'

beforeEach(() => server.use(...handlers))

/** settled is how long a wait lasts for the lazy inbox to load. */
const settled = { timeout: 3000 }

/**
 * Returns the router the plugin's routes build under a bare root.
 * @returns The router.
 */
function pluginRouter() {
	const rootRoute = createRootRoute()
	return createRouter({ routeTree: rootRoute.addChildren(plugin.routes(rootRoute)) })
}

test('loads the inbox in a chunk of its own, off the first paint', () => {
	const { component } = pluginRouter().routesById['/email'].options

	expect(typeof (component as { preload?: unknown }).preload).toBe('function')
})

test('keeps in the address only what a list view can hold', () => {
	const { validateSearch } = pluginRouter().routesById['/email'].options

	const kept = (validateSearch as (raw: Record<string, unknown>) => unknown)({
		page: 2,
		perPage: 'many',
		tab: 'other',
	})

	expect(kept).toEqual({ page: 2 })
})

test('serves the inbox at the address of its menu row', async () => {
	renderPluginAt(plugin, '/email', { session: adminSession })

	expect(await screen.findByRole('heading', { level: 2, name: 'Email' }, settled)).toBeInTheDocument()
	expect(await screen.findByRole('row', { name: /Delivery date/ }, settled)).toBeInTheDocument()
})

test('leads from the Email row to the inbox', async () => {
	const user = userEvent.setup()
	renderPluginAt(plugin, '/', { session: adminSession })

	const sidebar = await screen.findByRole('region', { name: 'Navigation' })
	await user.click(await within(sidebar).findByRole('link', { name: 'Email' }))

	expect(await screen.findByRole('heading', { level: 2, name: 'Email' }, settled)).toBeInTheDocument()
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { screen, within } from '@testing-library/react'
import { expect, test } from 'vitest'

import { createAppRouter } from '../router'
import { everyMenuItem, fullAccessSession } from './navSession'
import { renderAt, renderRoutedAt } from './render'

test('sends the root path to the day of tasks', async () => {
	const { router } = renderRoutedAt('/')

	expect(await screen.findByRole('heading', { level: 2, name: 'Tasks' })).toBeInTheDocument()
	expect(router.state.location.pathname).toBe('/tasks')
})

test('names the app in plain text at the head of the sidebar, as the Site Editor titles its own', async () => {
	renderAt('/')

	const nav = await screen.findByRole('region', { name: 'Navigation' })
	expect(within(nav).getByRole('heading', { level: 1, name: 'AlphOne' })).toBeInTheDocument()
	expect(screen.queryByRole('link', { name: 'AlphOne' })).toBeNull()
})

test('renders a navigation entry for every core and plugin section', async () => {
	renderAt('/', fullAccessSession)

	const nav = await screen.findByRole('region', { name: 'Navigation' })
	expect(within(within(nav).getByRole('list')).getAllByRole('link')).toHaveLength(everyMenuItem.length)
})

test('marks only entries that drill into a sidebar section with a chevron', async () => {
	renderAt('/')

	const nav = await screen.findByRole('region', { name: 'Navigation' })
	const users = within(nav).getByRole('link', { name: 'Users' })
	const whatsapp = within(nav).getByRole('link', { name: 'WhatsApp' })
	expect(users.querySelector('.godmin-navigation-item__drilldown-indicator')).toBeNull()
	expect(whatsapp.querySelector('.godmin-navigation-item__drilldown-indicator')).not.toBeNull()
})

test('frames the active route inside the main content region', async () => {
	renderAt('/')

	const main = await screen.findByRole('main')
	expect(within(main).getByRole('heading', { level: 2, name: 'Tasks' })).toBeInTheDocument()
	expect(within(main).queryByRole('region', { name: 'Navigation' })).toBeNull()
})

test('sets the router to preload a screen on intent, as the new WordPress admin does', () => {
	expect(createAppRouter().options.defaultPreload).toBe('intent')
})

test('sets no pending screen on the router or on any route', () => {
	const router = createAppRouter()

	expect(router.options.defaultPendingComponent).toBeUndefined()
	expect(Object.values(router.routesById).filter((route) => route.options.pendingComponent)).toEqual([])
})

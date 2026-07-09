// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Outlet,
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import { expect, test } from 'vitest'

import { plugins } from '../../plugins'
import { MainMenu } from '../MainMenu'

const navItems = plugins.flatMap((plugin) => plugin.nav)

function renderMenuAt(path: string) {
	const rootRoute = createRootRoute({
		component: function MenuHost() {
			return (
				<>
					<MainMenu />
					<Outlet />
				</>
			)
		},
	})
	const routes = [{ to: '/' }, ...navItems].map((item) =>
		createRoute({
			getParentRoute: () => rootRoute,
			path: item.to,
			component: function Blank() {
				return null
			},
		}),
	)
	const router = createRouter({
		routeTree: rootRoute.addChildren(routes),
		history: createMemoryHistory({ initialEntries: [path] }),
	})
	render(<RouterProvider router={router} />)
}

test('renders a menu link for every plugin nav entry', async () => {
	renderMenuAt('/')

	const nav = await screen.findByRole('navigation', { name: 'Main menu' })
	expect(within(nav).getAllByRole('link')).toHaveLength(navItems.length)
	for (const item of navItems) {
		expect(
			within(nav).getByRole('link', { name: item.label }),
		).toBeInTheDocument()
	}
})

test('marks the item for the active route as current', async () => {
	const [target] = navItems
	renderMenuAt(target.to)

	const link = await screen.findByRole('link', { name: target.label })
	expect(link).toHaveAttribute('aria-current', 'page')
})

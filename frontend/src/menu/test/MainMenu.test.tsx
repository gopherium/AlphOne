// SPDX-License-Identifier: AGPL-3.0-or-later

import { adminSession, seedSession } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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
import { coreNav } from '../coreNav'
import { MainMenu } from '../MainMenu'

const navItems = [...coreNav, ...plugins.flatMap((plugin) => plugin.nav)]

/**
 * Renders the main menu under a signed-in admin, the router standing at the path.
 * @param path - The route the router starts on.
 */
function renderMenuAt(path: string) {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false, staleTime: Infinity } },
	})
	seedSession(client, adminSession)
	const rootRoute = createRootRoute({
		component: function MenuHost() {
			return (
				<>
					<nav aria-label="Navigation">
						<MainMenu />
					</nav>
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
	render(
		<QueryClientProvider client={client}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	)
}

test('renders a menu link for every core and plugin nav entry', async () => {
	renderMenuAt('/')

	const nav = await screen.findByRole('navigation', { name: 'Navigation' })
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

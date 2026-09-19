// SPDX-License-Identifier: AGPL-3.0-or-later

import { memberSession, seedSession } from '@alphone/frontend-sdk/testing'
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
import { expect, test, vi } from 'vitest'

vi.mock('../../plugins', () => ({
	plugins: [
		{
			id: 'widgets',
			routes: () => [],
			nav: [
				{ label: 'Widgets', to: '/widgets', icon: <svg /> },
				{
					label: 'Widget settings',
					to: '/widget-settings',
					icon: <svg />,
					capability: 'manage_widgets',
				},
			],
		},
	],
}))

import { MainMenu } from '../MainMenu'

const managingSession = {
	...memberSession,
	capabilities: ['manage_widgets'],
}

/**
 * Renders the main menu under the signed-in account given.
 * @param session - The account the menu is rendered for.
 */
function renderMenuAs(session: typeof memberSession) {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false, staleTime: Infinity } },
	})
	seedSession(client, session)
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
	const routes = ['/', '/widgets', '/widget-settings'].map((path) =>
		createRoute({
			getParentRoute: () => rootRoute,
			path,
			component: function Blank() {
				return null
			},
		}),
	)
	const router = createRouter({
		routeTree: rootRoute.addChildren(routes),
		history: createMemoryHistory({ initialEntries: ['/'] }),
	})
	render(
		<QueryClientProvider client={client}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	)
}

test('leaves out a plugin entry whose capability the session lacks', async () => {
	renderMenuAs(memberSession)

	const nav = await screen.findByRole('navigation', { name: 'Navigation' })
	expect(within(nav).getByRole('link', { name: 'Widgets' })).toBeInTheDocument()
	expect(within(nav).queryByRole('link', { name: 'Widget settings' })).not.toBeInTheDocument()
})

test('shows a plugin entry whose capability the session holds', async () => {
	renderMenuAs(managingSession)

	const nav = await screen.findByRole('navigation', { name: 'Navigation' })
	expect(within(nav).getByRole('link', { name: 'Widgets' })).toBeInTheDocument()
	expect(within(nav).getByRole('link', { name: 'Widget settings' })).toBeInTheDocument()
})

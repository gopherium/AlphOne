// SPDX-License-Identifier: AGPL-3.0-or-later

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
	Link,
	Outlet,
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { cleanup, render } from '@testing-library/react'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, vi } from 'vitest'

import type { FrontendPlugin } from './index'

export const server = setupServer()

export function installTestEnvironment() {
	vi.stubGlobal('scrollTo', () => {})
	beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
	afterEach(() => {
		cleanup()
		server.resetHandlers()
	})
	afterAll(() => server.close())
}

export function renderPluginAt(plugin: FrontendPlugin, path: string) {
	const rootRoute = createRootRoute({
		component: function TestHost() {
			return (
				<>
					<nav>
						{plugin.nav.map((item) => (
							<Link key={item.to} to={item.to}>
								{item.label}
							</Link>
						))}
					</nav>
					<Outlet />
				</>
			)
		},
	})
	const homeRoute = createRoute({
		getParentRoute: () => rootRoute,
		path: '/',
		component: function TestHostHome() {
			return <p>Test host home</p>
		},
	})
	const routeTree = rootRoute.addChildren([
		homeRoute,
		...plugin.routes(rootRoute),
	])
	const router = createRouter({
		routeTree,
		history: createMemoryHistory({ initialEntries: [path] }),
	})
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	})
	render(
		<QueryClientProvider client={client}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	)
}

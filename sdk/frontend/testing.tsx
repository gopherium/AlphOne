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
	useRouterState,
} from '@tanstack/react-router'
import { cleanup, render } from '@testing-library/react'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, vi } from 'vitest'

import type { FrontendPlugin } from './index'

export { http, HttpResponse } from 'msw'

/**
 * FakeEventSource stands in for the browser EventSource, which jsdom does
 * not implement. Tests drive it synchronously via emit().
 */
export class FakeEventSource {
	static readonly CONNECTING = 0
	static readonly OPEN = 1
	static readonly CLOSED = 2

	static instances: FakeEventSource[] = []

	/**
	 * Resets the list of tracked FakeEventSource instances.
	 */
	static reset() {
		FakeEventSource.instances = []
	}

	/**
	 * Returns the most recently created FakeEventSource instance.
	 * @returns The last created instance, throwing if none exists.
	 */
	static last() {
		const source = FakeEventSource.instances.at(-1)
		if (!source) {
			throw new Error('no EventSource was created')
		}
		return source
	}

	url: string
	onmessage: ((event: MessageEvent) => void) | null = null
	onopen: ((event: Event) => void) | null = null
	onerror: ((event: Event) => void) | null = null
	readyState: number = FakeEventSource.CONNECTING
	closed = false

	/**
	 * Creates a FakeEventSource for the given URL and records the instance.
	 * @param url - The URL the EventSource would connect to.
	 */
	constructor(url: string) {
		this.url = url
		FakeEventSource.instances.push(this)
	}

	/**
	 * Marks this EventSource as closed.
	 */
	close() {
		this.closed = true
		this.readyState = FakeEventSource.CLOSED
	}

	/**
	 * Dispatches an empty JSON message event to the registered handler.
	 */
	emit() {
		this.onmessage?.(new MessageEvent('message', { data: '{}' }))
	}

	/**
	 * Marks the connection open and dispatches an open event to the registered handler.
	 */
	emitOpen() {
		this.readyState = FakeEventSource.OPEN
		this.onopen?.(new Event('open'))
	}

	/**
	 * Records the connection state the browser would leave after a failure and
	 * dispatches an error event to the registered handler.
	 * @param readyState - The state the connection is in once the error fires.
	 */
	emitError(readyState: number) {
		this.readyState = readyState
		this.onerror?.(new Event('error'))
	}
}

export const server = setupServer()

/**
 * Installs global stubs and vitest lifecycle hooks for the test environment.
 */
export function installTestEnvironment() {
	vi.stubGlobal('scrollTo', () => {})
	vi.stubGlobal('EventSource', FakeEventSource)
	beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
	afterEach(() => {
		cleanup()
		server.resetHandlers()
		FakeEventSource.reset()
	})
	afterAll(() => server.close())
}

/**
 * Renders the given frontend plugin mounted at a specific route path.
 * @param plugin - The frontend plugin whose nav and routes are mounted.
 * @param path - The initial router path to render at.
 */
export function renderPluginAt(plugin: FrontendPlugin, path: string) {
	const rootRoute = createRootRoute({
		component: function TestHost() {
			const matches = useRouterState({ select: (state) => state.matches })
			const sidebarMatch = [...matches]
				.reverse()
				.find((match) => match.staticData.Sidebar)
			const Sidebar = sidebarMatch?.staticData.Sidebar
			return (
				<>
					<nav aria-label="Navigation">
						{Sidebar ? (
							<Sidebar />
						) : (
							plugin.nav.map((item) => (
								<Link key={item.to} to={item.to}>
									{item.label}
								</Link>
							))
						)}
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

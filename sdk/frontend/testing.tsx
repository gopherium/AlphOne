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

/**
 * FakeEventSource stands in for the browser EventSource, which jsdom does
 * not implement. Tests drive it synchronously via emit().
 */
export class FakeEventSource {
	static instances: FakeEventSource[] = []

	static reset() {
		FakeEventSource.instances = []
	}

	static last() {
		const source = FakeEventSource.instances.at(-1)
		if (!source) {
			throw new Error('no EventSource was created')
		}
		return source
	}

	url: string
	onmessage: ((event: MessageEvent) => void) | null = null
	closed = false

	constructor(url: string) {
		this.url = url
		FakeEventSource.instances.push(this)
	}

	close() {
		this.closed = true
	}

	emit() {
		this.onmessage?.(new MessageEvent('message', { data: '{}' }))
	}
}

export const server = setupServer()

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

// SPDX-License-Identifier: AGPL-3.0-or-later

import '@testing-library/jest-dom/vitest'
import { installTestEnvironment as installAdminTestEnvironment } from '@gopherium/godmin/testing'
import { installTestEnvironment as installAuthTestEnvironment } from '@gopherium/react-auth/testing'
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
import { act, render } from '@testing-library/react'
import { Client, fetchExchange, subscriptionExchange } from 'urql'
import { vi } from 'vitest'

import { graphCacheExchange } from './graph'
import type { GraphClient } from './graph'
import { GraphProvider } from './GraphProvider'
import type { FrontendPlugin } from './index'

export { HttpResponse, http, server } from '@gopherium/react-auth/testing'
export { graphql } from 'msw'

/** FakeGraph drives a graph client's subscriptions from a test. */
export interface FakeGraph {
	/** graph stands in for the client the screens and plugins consume. */
	graph: GraphClient
	/** documents lists every subscription document the client forwarded. */
	documents: string[]
	/** unsubscribes counts the subscriptions torn down so far. */
	unsubscribes: () => number
	/** emit delivers one frame to the newest subscription. */
	emit: (data: Record<string, unknown>) => void
	/** openStream announces an event stream connection to its listeners. */
	openStream: () => void
}

/**
 * Returns a graph client whose subscription frames and connections a test drives.
 * @returns The fake client beside the controls driving it.
 */
export function fakeGraphClient(): FakeGraph {
	const sinks: { next: (value: { data: Record<string, unknown> }) => void }[] = []
	const documents: string[] = []
	const listeners = new Set<() => void>()
	let torn = 0
	const client = new Client({
		url: '/api/graphql',
		fetchOptions: { credentials: 'same-origin' },
		preferGetMethod: false,
		exchanges: [
			graphCacheExchange(),
			subscriptionExchange({
				forwardSubscription: (request) => {
					documents.push(String(request.query))
					return {
						subscribe: (sink) => {
							sinks.push(sink)
							return {
								unsubscribe: () => {
									torn += 1
								},
							}
						},
					}
				},
			}),
			fetchExchange,
		],
	})
	return {
		graph: {
			client,
			refetch: vi.fn(),
			onStreamOpen: (listener) => {
				listeners.add(listener)
				return () => {
					listeners.delete(listener)
				}
			},
		},
		documents,
		unsubscribes: () => torn,
		emit: (data) =>
			act(() => {
				sinks.at(-1)?.next({ data })
			}),
		openStream: () =>
			act(() => {
				for (const listener of listeners) {
					listener()
				}
			}),
	}
}

/**
 * Installs global stubs and vitest lifecycle hooks for the test environment.
 */
export function installTestEnvironment() {
	vi.stubGlobal('scrollTo', () => {})
	installAdminTestEnvironment()
	installAuthTestEnvironment()
}

/**
 * Renders the given frontend plugin mounted at a specific route path.
 * @param plugin - The frontend plugin whose nav and routes are mounted.
 * @param path - The initial router path to render at.
 * @returns The fake graph client the mounted plugin consumes.
 */
export function renderPluginAt(plugin: FrontendPlugin, path: string): FakeGraph {
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
	const fake = fakeGraphClient()
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={fake.graph}>
				<RouterProvider router={router} />
			</GraphProvider>
		</QueryClientProvider>,
	)
	return fake
}

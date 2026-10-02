// SPDX-License-Identifier: AGPL-3.0-or-later

import '@testing-library/jest-dom/vitest'
import { Toaster } from '@gopherium/godmin'
import { installTestEnvironment as installAdminTestEnvironment } from '@gopherium/godmin/testing'
import {
	HttpResponse,
	installTestEnvironment as installAuthTestEnvironment,
	defaultUser,
	server,
} from '@gopherium/react-auth/testing'
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
import { Badge, Button, Text } from '@wordpress/ui'
import { graphql } from 'msw'
import type { ComponentProps, ReactElement } from 'react'
import { Client, fetchExchange, subscriptionExchange } from 'urql'
import { vi } from 'vitest'

import { doorbellExchange, graphCacheExchange, graphRetryExchange } from './graph'
import type { GraphClient } from './graph'
import { GraphProvider } from './GraphProvider'
import type { FrontendPlugin } from './index'

export { HttpResponse, http, seedSession, server } from '@gopherium/react-auth/testing'
export { setViewport } from '@gopherium/godmin/testing'
export { resetLocaleData, setLocaleData } from '@wordpress/i18n'
export { delay, graphql } from 'msw'

/** adminSession is the canned signed-in account holding the admin role. */
export const adminSession = {
	...defaultUser,
	role: 'admin',
	capabilities: ['manage_users'],
	grantable: ['admin', 'member'],
}

/** memberSession is the canned signed-in account holding the member role. */
export const memberSession = {
	...defaultUser,
	role: 'member',
	capabilities: [] as string[],
	grantable: ['member'],
}

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
	const doorbell = doorbellExchange()
	const client = new Client({
		url: '/api/graphql',
		fetchOptions: { credentials: 'same-origin' },
		preferGetMethod: false,
		exchanges: [
			doorbell.exchange,
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
			graphRetryExchange(),
			fetchExchange,
		],
	})
	return {
		graph: {
			client,
			refetch: vi.fn(doorbell.refetch),
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
 * Returns the classes the outer element of a rendered tree carries.
 * @param tree - The tree to sample.
 * @returns The class names, in order.
 */
function classesOf(tree: ReactElement): string[] {
	const { container, unmount } = render(tree)
	const classes = [...(container.firstElementChild as Element).classList]
	unmount()
	return classes
}

/**
 * Returns the classes a Text renders at the given variant.
 * @param variant - The text variant to sample.
 * @returns The class names, in order.
 */
export function textClasses(variant: ComponentProps<typeof Text>['variant']): string[] {
	return classesOf(<Text variant={variant} />)
}

/**
 * Returns the classes a Badge renders at the given intent.
 * @param intent - The badge intent to sample.
 * @returns The class names, in order.
 */
export function badgeClasses(intent: ComponentProps<typeof Badge>['intent']): string[] {
	return classesOf(<Badge intent={intent}>probe</Badge>)
}

/**
 * Returns the classes a Button renders at the given variant and size.
 * @param variant - The button variant to sample.
 * @param size - The button size, the default one when absent.
 * @returns The class names, in order.
 */
export function buttonClasses(
	variant: ComponentProps<typeof Button>['variant'],
	size?: ComponentProps<typeof Button>['size'],
): string[] {
	return classesOf(
		<Button variant={variant} size={size}>
			probe
		</Button>,
	)
}

/**
 * Serves admin settings whose page sizes are small enough to page a short list.
 * @param sizes - The page sizes a list offers.
 * @param size - The page size a list opens on.
 */
export function paging(sizes: number[], size: number) {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds: 6000,
						listPageSizes: sizes,
						listPageSize: size,
						contactPageCap: 200,
						formatLocale: 'es-ES',
					},
				},
			}),
		),
	)
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
 * Renders the given frontend plugin mounted at a specific route path, below a toaster.
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
				<Toaster>
					<RouterProvider router={router} />
				</Toaster>
			</GraphProvider>
		</QueryClientProvider>,
	)
	return fake
}

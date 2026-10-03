// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, Toaster } from '@alphone/frontend-sdk'
import {
	adminSession,
	fakeGraphClient,
	resetLocaleData,
	seedSession,
	setLocaleData,
} from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { render } from '@testing-library/react'
import type { ReactNode } from 'react'
import { onTestFinished } from 'vitest'

import { DOMAIN, plugin } from '../index'
import { routes } from '../routes'

/** Loads the importer's Spanish catalogue until the test finishes. */
export async function inSpanish() {
	setLocaleData(await plugin.locale?.load('es-ES'), DOMAIN)
	onTestFinished(() => resetLocaleData(undefined, DOMAIN))
}

/**
 * Renders a tree for a signed-in admin inside the query, graph and toast providers.
 * @param tree - The tree to render.
 */
export function renderHosted(tree: ReactNode) {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
	seedSession(client, adminSession)
	const { graph } = fakeGraphClient()
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<Toaster>{tree}</Toaster>
			</GraphProvider>
		</QueryClientProvider>,
	)
}

/**
 * Renders the importer routes at the given path for a signed-in admin.
 * @param path - The path to start at.
 * @returns The router, to read where a test ended up.
 */
export function renderAt(path: string) {
	const rootRoute = createRootRoute()
	const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: '/' })
	const router = createRouter({
		routeTree: rootRoute.addChildren([indexRoute, ...routes(rootRoute)]),
		history: createMemoryHistory({ initialEntries: [path] }),
	})
	renderHosted(<RouterProvider router={router as never} />)
	return router
}

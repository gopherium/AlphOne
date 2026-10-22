// SPDX-License-Identifier: AGPL-3.0-or-later

import { MANAGE_USERS } from '@alphone/frontend-sdk'
import type { Capability } from '@alphone/frontend-sdk'
import { adminSession, seedSession } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { CapabilityGate } from '../CapabilityGate'

/**
 * Renders a signed-in admin at a route behind the gate, the route declaring the capability given.
 * @param capability - What the gated route declares, or nothing.
 */
function renderGated(capability?: Capability) {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
	seedSession(client, adminSession)
	const rootRoute = createRootRoute()
	const homeRoute = createRoute({
		getParentRoute: () => rootRoute,
		path: '/',
		component: function Home() {
			return <h1>{'Home'}</h1>
		},
	})
	const gateRoute = createRoute({ getParentRoute: () => rootRoute, id: 'gate', component: CapabilityGate })
	const gatedRoute = createRoute({
		getParentRoute: () => gateRoute,
		path: '/gated',
		staticData: capability === undefined ? {} : { capability },
		component: function Gated() {
			return <h1>{'Gated'}</h1>
		},
	})
	const router = createRouter({
		routeTree: rootRoute.addChildren([homeRoute, gateRoute.addChildren([gatedRoute])]),
		history: createMemoryHistory({ initialEntries: ['/gated'] }),
	})
	render(
		<QueryClientProvider client={client}>
			<RouterProvider router={router as never} />
		</QueryClientProvider>,
	)
}

test('opens a gated route to a session holding the capability it declares', async () => {
	renderGated(MANAGE_USERS)

	expect(await screen.findByRole('heading', { name: 'Gated' })).toBeInTheDocument()
})

test('sends the reader home from a gated route that declares no capability', async () => {
	renderGated()

	expect(await screen.findByRole('heading', { name: 'Home' })).toBeInTheDocument()
	expect(screen.queryByRole('heading', { name: 'Gated' })).not.toBeInTheDocument()
})

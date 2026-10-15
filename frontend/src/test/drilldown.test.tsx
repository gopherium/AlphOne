// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider } from '@alphone/frontend-sdk'
import {
	HttpResponse,
	adminSession,
	fakeGraphClient,
	graphql,
	seedSession,
	server,
} from '@alphone/frontend-sdk/testing'
import { createAuthQueryClient } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import {
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import { lazy } from 'react'
import type { ComponentType } from 'react'
import { beforeEach, expect, test } from 'vitest'

import { RailContent } from '../RailContent'
import { renderAt } from './render'

/**
 * Renders the rail alone at a section whose sidebar is the given component, for a signed-in admin.
 * @param Sidebar - The section sidebar the rail shows.
 */
function renderRail(Sidebar: ComponentType) {
	const rootRoute = createRootRoute({ component: RailContent })
	const sectionRoute = createRoute({ getParentRoute: () => rootRoute, path: '/', staticData: { Sidebar } })
	const router = createRouter({
		routeTree: rootRoute.addChildren([sectionRoute]),
		history: createMemoryHistory({ initialEntries: ['/'] }),
	})
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, adminSession)
	server.use(graphql.query('Version', () => HttpResponse.json({ data: { version: '0.1.0' } })))
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={fakeGraphClient().graph}>
				<RouterProvider router={router as never} />
			</GraphProvider>
		</QueryClientProvider>,
	)
}

beforeEach(() =>
	server.use(
		graphql.query('WhatsAppConversations', () =>
			HttpResponse.json({
				data: {
					whatsAppConversations: [
						{
							__typename: 'WhatsAppConversation',
							id: '019f4a00-0000-7000-8000-000000000001',
							status: 'open',
							lastActivityAt: '2026-07-06T10:05:00Z',
							lastMessagePreview: 'The analytical engine awaits.',
							contact: {
								__typename: 'Contact',
								id: '019f4a00-0000-7000-8000-0000000000a1',
								name: 'Ada Lovelace',
							},
						},
					],
				},
			}),
		),
	),
)

test('drills the sidebar into the WhatsApp section screen', async () => {
	renderAt('/whatsapp')

	expect(
		await screen.findByRole('heading', { name: 'WhatsApp' }),
	).toBeInTheDocument()
	expect(screen.getByRole('link', { name: 'Back' })).toBeInTheDocument()
	expect(await screen.findByText('Ada Lovelace')).toBeInTheDocument()
	expect(
		within(screen.getByRole('main')).getByText('No conversation selected.'),
	).toBeInTheDocument()
	expect(
		screen.getByRole('navigation', { name: 'Navigation' }),
	).toBeInTheDocument()
})

test('keeps the brand and the version in the rail while a section sidebar loads', async () => {
	renderRail(lazy(() => new Promise<never>(() => {})))

	expect(await screen.findByRole('link', { name: 'AlphOne' })).toBeInTheDocument()
	expect(await screen.findByText('v0.1.0')).toBeInTheDocument()
	expect(screen.getByRole('navigation', { name: 'Navigation' })).toBeEmptyDOMElement()
})

test('shows the main menu, not a section screen, at the root', async () => {
	renderAt('/')

	expect(
		await screen.findByRole('navigation', { name: 'Navigation' }),
	).toBeInTheDocument()
	expect(screen.queryByRole('heading', { name: 'WhatsApp' })).toBeNull()
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, createGraphClient } from '@alphone/frontend-sdk'
import { HttpResponse, adminSession, graphql, http, server } from '@alphone/frontend-sdk/testing'
import { configureAuthTransport, createAuthQueryClient, sessionQueryKey } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { seedSession } from '@gopherium/react-auth/testing'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { act, render } from '@testing-library/react'

import { graphAuthTransport } from '../auth/graphTransport'
import { createAppRouter } from '../router'

configureAuthTransport(graphAuthTransport)

/**
 * Serves the core event subscription from a stream the test pushes frames into.
 * @returns The announcer delivering one event name to the subscribed app.
 */
export function liveStream() {
	let ready: (controller: ReadableStreamDefaultController<Uint8Array>) => void
	const connected = new Promise<ReadableStreamDefaultController<Uint8Array>>((resolve) => {
		ready = resolve
	})
	const encoder = new TextEncoder()
	server.use(
		http.post('/api/graphql', async ({ request }) => {
			const body = (await request.clone().json()) as { query?: string }
			if (!body.query?.includes('subscription')) {
				return undefined
			}
			return new HttpResponse(
				new ReadableStream({
					start: (controller) => {
						ready(controller)
					},
				}),
				{ headers: { 'content-type': 'text/event-stream' } },
			)
		}),
	)
	return {
		announce: async (name: string) => {
			const controller = await connected
			const frame = `event: next\ndata: ${JSON.stringify({ data: { coreEvent: name } })}\n\n`
			await act(async () => {
				controller.enqueue(encoder.encode(frame))
				await new Promise((resolve) => setTimeout(resolve, 0))
			})
		},
	}
}

/**
 * Renders the app at one route with a seeded session.
 * @param path - The route the memory history starts on.
 * @param user - The signed-in account, or null for an anonymous caller.
 * @param version - The version the graph answers, or null to make it fail.
 * @returns The query client the render used.
 */
export function renderAt(
	path: string,
	user: (User & { role?: string }) | null = adminSession,
	version: string | null = '0.1.0',
) {
	const client = createAuthQueryClient({
		queries: { retry: false, staleTime: Infinity },
	})
	seedSession(client, user)
	const graph = createGraphClient({
		onSessionExpired: () => client.setQueryData(sessionQueryKey, null),
	})
	server.use(
		graphql.query('Version', () =>
			version === null
				? HttpResponse.json({ data: null, errors: [{ message: 'the version is unavailable' }] })
				: HttpResponse.json({ data: { version } }),
		),
	)
	const router = createAppRouter(
		createMemoryHistory({ initialEntries: [path] }),
	)
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<RouterProvider router={router} />
			</GraphProvider>
		</QueryClientProvider>,
	)
	return client
}

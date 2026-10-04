// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, GraphProvider, createGraphClient } from '@alphone/frontend-sdk'
import {
	HttpResponse,
	adminSession,
	badgeClasses,
	buttonClasses,
	graphql,
	http,
	server,
} from '@alphone/frontend-sdk/testing'
import { configureAuthTransport, createAuthQueryClient, sessionQueryKey } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { seedSession } from '@gopherium/react-auth/testing'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { act, render } from '@testing-library/react'

import { AppToaster } from '../AppToaster'
import { graphAuthTransport } from '../auth/graphTransport'
import { configureAppChannelNames } from '../contacts/channel'
import { createAppRouter } from '../router'

export { badgeClasses, buttonClasses }

configureAuthTransport(graphAuthTransport)
configureAppChannelNames()

/**
 * Returns the class tokens the design system adds to a compact button.
 * @returns The tokens a compact button carries and a default one does not.
 */
export function compactClasses(): string[] {
	const roomy = new Set(buttonClasses('solid'))
	return buttonClasses('solid', 'compact').filter((token) => !roomy.has(token))
}

/**
 * Returns the class tokens the design system adds to a loading button.
 * @returns The tokens a busy button carries and an idle one does not.
 */
export function busyClasses(): string[] {
	const { container, unmount } = render(
		<>
			<Button id="idle">idle</Button>
			<Button id="busy" loading>
				busy
			</Button>
		</>,
	)
	const idle = new Set((container.querySelector('#idle') as Element).classList)
	const busy = [...(container.querySelector('#busy') as Element).classList]
	unmount()
	return busy.filter((token) => !idle.has(token))
}

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
 * Renders the app at one route with a seeded session, below the app toaster.
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
				<AppToaster>
					<RouterProvider router={router} />
				</AppToaster>
			</GraphProvider>
		</QueryClientProvider>,
	)
	return client
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import { adoptSession, createAuthQueryClient, sessionQueryKey } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { GraphProvider, createGraphClient, useHiddenPlugins } from '../index'
import type { GraphClient } from '../index'
import { HttpResponse, adminSession, graphql, memberSession, seedSession, server } from '../testing'

/** Shows what the hidden plugins hook answers. */
function Probe() {
	return <output aria-label="hidden plugins">{JSON.stringify(useHiddenPlugins())}</output>
}

/**
 * Renders one more probe below the given query client and graph client.
 * @param client - The query client holding the session.
 * @param graph - The graph client the probe reads through.
 */
function mountProbe(client: QueryClient, graph: GraphClient) {
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<Probe />
			</GraphProvider>
		</QueryClientProvider>,
	)
}

/**
 * Renders the probe below a graph client, signed in as the given account.
 * @param user - The signed-in account, or null for nobody.
 * @param graph - The graph client the probe reads through.
 * @returns The query client holding the session.
 */
function renderProbe(
	user: (User & { role?: string }) | null,
	graph: GraphClient = createGraphClient({ onSessionExpired: () => {} }),
): QueryClient {
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, user)
	mountProbe(client, graph)
	return client
}

/**
 * Returns what the probe shows.
 * @returns The parsed answer.
 */
function shown() {
	return JSON.parse(screen.getByRole('status', { name: 'hidden plugins' }).textContent ?? '') as unknown
}

/**
 * Serves the given ids as the plugins that hide their screens, counting each read.
 * @param ids - The ids the graph answers.
 * @returns The count of reads so far.
 */
function serveHidden(ids: string[]): () => number {
	let asked = 0
	server.use(
		graphql.query('HiddenPlugins', () => {
			asked += 1
			return HttpResponse.json({ data: { hiddenPlugins: ids } })
		}),
	)
	return () => asked
}

test('hands a signed-in screen the ids of the plugins that hide their screens', async () => {
	serveHidden(['email', 'whatsapp'])
	renderProbe(adminSession)

	await waitFor(() => expect(shown()).toEqual(['email', 'whatsapp']))
})

test('answers no hidden plugin while the read loads', async () => {
	let asked = 0
	server.use(
		graphql.query('HiddenPlugins', () => {
			asked += 1
			return new Promise<never>(() => {})
		}),
	)
	renderProbe(adminSession)

	await waitFor(() => expect(asked).toBe(1))
	expect(shown()).toEqual([])
})

test('reads again for the next account that signs in', async () => {
	serveHidden(['email'])
	const client = renderProbe(adminSession)
	await waitFor(() => expect(shown()).toEqual(['email']))

	const asked = serveHidden(['whatsapp'])
	await act(() => adoptSession(client, memberSession))

	await waitFor(() => expect(shown()).toEqual(['whatsapp']))
	expect(asked()).toBe(1)
})

test('answers no hidden plugin for the next account when its read fails, though the last read named one', async () => {
	serveHidden(['email'])
	const client = renderProbe(adminSession)
	await waitFor(() => expect(shown()).toEqual(['email']))

	let failed = 0
	server.use(
		graphql.query('HiddenPlugins', () => {
			failed += 1
			return HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })
		}),
	)
	await act(() => adoptSession(client, memberSession))

	await waitFor(() => expect(failed).toBe(1))
	await waitFor(() => expect(shown()).toEqual([]))
})

test('reads again when the session turns to another account without a sign-in', async () => {
	serveHidden(['email'])
	const client = renderProbe(adminSession)
	await waitFor(() => expect(shown()).toEqual(['email']))

	const asked = serveHidden(['whatsapp'])
	act(() => client.setQueryData(sessionQueryKey, memberSession))

	await waitFor(() => expect(shown()).toEqual(['whatsapp']))
	expect(asked()).toBe(1)
})

test('reads apart for the next account that signs in while the last read still runs', async () => {
	let release = () => {}
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	let asked = 0
	server.use(
		graphql.query('HiddenPlugins', async () => {
			asked += 1
			if (asked === 1) {
				await held
				return HttpResponse.json({ data: { hiddenPlugins: ['email'] } })
			}
			return HttpResponse.json({ data: { hiddenPlugins: ['whatsapp'] } })
		}),
	)
	const client = renderProbe(adminSession)
	await waitFor(() => expect(asked).toBe(1))

	await act(() => adoptSession(client, memberSession))

	await waitFor(() => expect(shown()).toEqual(['whatsapp']))
	release()
	expect(asked).toBe(2)
})

test('shares one read with a screen that asks after the answer came', async () => {
	const asked = serveHidden(['email'])
	const graph = createGraphClient({ onSessionExpired: () => {} })
	const client = renderProbe(adminSession, graph)
	client.setQueryDefaults(['hiddenPlugins'], { staleTime: 0 })
	await waitFor(() => expect(shown()).toEqual(['email']))

	mountProbe(client, graph)

	const answers = screen.getAllByRole('status', { name: 'hidden plugins' }).map((probe) => probe.textContent)
	expect(answers).toEqual(['["email"]', '["email"]'])
	expect(client.isFetching()).toBe(0)
	expect(asked()).toBe(1)
})

test('asks nothing before anyone signs in', () => {
	const graph = createGraphClient({ onSessionExpired: () => {} })
	const asks = vi.spyOn(graph.client, 'executeQuery')
	renderProbe(null, graph)

	expect(shown()).toEqual([])
	expect(asks).not.toHaveBeenCalled()
})

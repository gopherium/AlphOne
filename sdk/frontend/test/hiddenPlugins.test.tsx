// SPDX-License-Identifier: AGPL-3.0-or-later

import { createAuthQueryClient } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { GraphProvider, createGraphClient, useHiddenPlugins } from '../index'
import type { GraphClient } from '../index'
import { HttpResponse, adminSession, graphql, seedSession, server } from '../testing'

/** Shows what the hidden plugins hook answers. */
function Probe() {
	return <output aria-label="hidden plugins">{JSON.stringify(useHiddenPlugins())}</output>
}

/**
 * Renders the probe below a graph client, signed in as the given account.
 * @param user - The signed-in account, or null for nobody.
 * @param graph - The graph client the probe reads through.
 * @returns The graph client the probe reads through.
 */
function renderProbe(
	user: (User & { role?: string }) | null,
	graph: GraphClient = createGraphClient({ onSessionExpired: () => {} }),
): GraphClient {
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, user)
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<Probe />
			</GraphProvider>
		</QueryClientProvider>,
	)
	return graph
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

test('answers no hidden plugin once a read fails, though an earlier read named one', async () => {
	serveHidden(['email'])
	const graph = renderProbe(adminSession)
	await waitFor(() => expect(shown()).toEqual(['email']))

	server.use(
		graphql.query('HiddenPlugins', () => HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })),
	)
	act(() => graph.refetch(['HiddenPlugins']))

	await waitFor(() => expect(shown()).toEqual([]))
})

test('asks nothing before anyone signs in', () => {
	const graph = createGraphClient({ onSessionExpired: () => {} })
	const asks = vi.spyOn(graph.client, 'executeQuery')
	renderProbe(null, graph)

	expect(shown()).toEqual([])
	expect(asks).not.toHaveBeenCalled()
})

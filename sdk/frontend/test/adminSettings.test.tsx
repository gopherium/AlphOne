// SPDX-License-Identifier: AGPL-3.0-or-later

import { AuthGate, createAuthQueryClient, sessionQueryKey } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import { useWriteTimeout } from '../adminSettings'
import { GraphProvider, createGraphClient, useAdminSettings } from '../index'
import { HttpResponse, adminSession, delay, graphql, memberSession, seedSession, server } from '../testing'

/** servedSettings are the settings the graph serves in these tests. */
const servedSettings = {
	toastMilliseconds: 8000,
	toastActionMilliseconds: 12000,
	dialogWriteTimeoutMilliseconds: 45000,
	listPageSizes: [5, 25],
	listPageSize: 25,
	contactPageCap: 150,
	formatLocale: 'de-DE',
}

/** Shows what the settings hook answers. */
function Probe() {
	return <output aria-label="settings">{JSON.stringify(useAdminSettings())}</output>
}

/** Shows the write time limit a busy write gets. */
function TimeoutProbe() {
	return <output aria-label="write timeout">{String(useWriteTimeout())}</output>
}

/**
 * Returns a query client signed in as the admin, the session never stale.
 * @returns The query client holding the session.
 */
function signedInClient(): QueryClient {
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, adminSession)
	return client
}

/**
 * Renders the given tree below a graph client, signed in as the admin.
 * @param tree - The screens reading the settings.
 * @returns The query client holding the session.
 */
function renderProbe(tree: ReactNode = <Probe />): QueryClient {
	const client = signedInClient()
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>{tree}</GraphProvider>
		</QueryClientProvider>,
	)
	return client
}

/**
 * Serves the settings of these tests, counting each read.
 * @returns The count of reads so far.
 */
function serveSettings(): () => number {
	let asked = 0
	server.use(
		graphql.query('AdminSettings', () => {
			asked += 1
			return HttpResponse.json({ data: { adminSettings: { __typename: 'AdminSettings', ...servedSettings } } })
		}),
	)
	return () => asked
}

/**
 * Returns what the probe shows once it holds the given text.
 * @param text - A part of the answer to wait for.
 * @returns The parsed answer.
 */
async function answered(text: RegExp) {
	return JSON.parse((await screen.findByText(text)).textContent ?? '') as unknown
}

test('hands a signed-in screen the settings the graph serves', async () => {
	serveSettings()
	renderProbe()

	expect(await answered(/toastMilliseconds/)).toEqual({ settings: servedSettings, failed: false })
})

test('reports settings the graph could not serve', async () => {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderProbe()

	expect(await answered(/"failed":true/)).toEqual({ failed: true })
})

test('reads the settings again for the next account that signs in', async () => {
	const asked = serveSettings()
	const client = renderProbe(
		<AuthGate loginScreen={(onLogin) => <button onClick={() => onLogin(memberSession)}>Log in</button>}>
			<Probe />
		</AuthGate>,
	)
	await answered(/toastMilliseconds/)

	act(() => client.setQueryData(sessionQueryKey, null))
	fireEvent.click(await screen.findByRole('button', { name: 'Log in' }))

	await answered(/toastMilliseconds/)
	await waitFor(() => expect(asked()).toBe(2))
})

test('shares one read between every screen that asks', async () => {
	const asked = serveSettings()
	renderProbe(
		<>
			<Probe />
			<Probe />
		</>,
	)

	await waitFor(() => expect(screen.getAllByText(/toastMilliseconds/)).toHaveLength(2))
	expect(asked()).toBe(1)
})

test('hands the write time limit a screen loaded, asking the graph nothing itself', async () => {
	const asked = serveSettings()
	render(
		<QueryClientProvider client={signedInClient()}>
			<TimeoutProbe />
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>
				<Probe />
			</GraphProvider>
		</QueryClientProvider>,
	)

	expect(await screen.findByText('45000')).toBeInTheDocument()
	expect(asked()).toBe(1)
})

test('answers no write time limit until a screen loaded the settings', async () => {
	const asked = serveSettings()
	render(
		<QueryClientProvider client={signedInClient()}>
			<TimeoutProbe />
		</QueryClientProvider>,
	)

	expect(screen.getByRole('status', { name: 'write timeout' })).toHaveTextContent('Infinity')
	await delay(50)
	expect(asked()).toBe(0)
})

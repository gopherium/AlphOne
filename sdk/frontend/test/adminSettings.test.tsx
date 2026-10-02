// SPDX-License-Identifier: AGPL-3.0-or-later

import { createAuthQueryClient } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { GraphProvider, createGraphClient, useAdminSettings } from '../index'
import { HttpResponse, adminSession, graphql, seedSession, server } from '../testing'

/** Shows what the settings hook answers. */
function Probe() {
	return <output aria-label="settings">{JSON.stringify(useAdminSettings())}</output>
}

/**
 * Renders the probe below a graph client, signed in as the given account.
 * @param user - The signed-in account, or null for nobody.
 */
function renderProbe(user: (User & { role?: string }) | null) {
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, user)
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>
				<Probe />
			</GraphProvider>
		</QueryClientProvider>,
	)
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
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds: 8000,
						listPageSizes: [5, 25],
						listPageSize: 25,
						contactPageCap: 150,
						formatLocale: 'de-DE',
					},
				},
			}),
		),
	)
	renderProbe(adminSession)

	expect(await answered(/toastMilliseconds/)).toEqual({
		settings: {
			toastMilliseconds: 8000,
			listPageSizes: [5, 25],
			listPageSize: 25,
			contactPageCap: 150,
			formatLocale: 'de-DE',
		},
		failed: false,
	})
})

test('asks nothing before anyone signs in', async () => {
	let asked = 0
	server.use(
		graphql.query('AdminSettings', () => {
			asked += 1
			return HttpResponse.json({ data: null, errors: [{ message: 'no session' }] })
		}),
	)
	renderProbe(null)

	expect(await answered(/failed/)).toEqual({ failed: false })
	expect(asked).toBe(0)
})

test('reports settings the graph could not serve', async () => {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderProbe(adminSession)

	expect(await answered(/"failed":true/)).toEqual({ failed: true })
})

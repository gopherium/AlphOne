// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberLocale } from '@gopherium/gottext'
import { resetLocale } from '@gopherium/gottext/testing'
import { createAuthQueryClient } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { FormatLocaleGate, GraphProvider, createGraphClient, formatDate, rememberFormatLocale } from '../index'
import { HttpResponse, adminSession, delay, graphql, seedSession, server } from '../testing'

afterEach(() => {
	rememberFormatLocale(undefined)
	resetLocale()
})

/** Shows one date as the screens write it. */
function Dated() {
	return <p>{formatDate(new Date(2026, 8, 30))}</p>
}

/** Renders the gate around a dated screen, signed in, the loading text standing in until it opens. */
function renderGate() {
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, adminSession)
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>
				<FormatLocaleGate loading={<p>Loading settings</p>}>
					<Dated />
				</FormatLocaleGate>
			</GraphProvider>
		</QueryClientProvider>,
	)
}

/**
 * Serves the admin settings naming the given format locale, after a pause.
 * @param formatLocale - The locale the settings name.
 */
function serveSettings(formatLocale: string) {
	server.use(
		graphql.query('AdminSettings', async () => {
			await delay(50)
			return HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds: 6000,
						listPageSizes: [10, 20],
						listPageSize: 20,
						contactPageCap: 200,
						formatLocale,
					},
				},
			})
		}),
	)
}

test('holds the screens back until the server names the format locale', async () => {
	rememberFormatLocale(undefined)
	rememberLocale('en-US')
	serveSettings('de-DE')

	renderGate()

	expect(screen.getByText('Loading settings')).toBeInTheDocument()
	expect(await screen.findByText('30.09.2026')).toBeInTheDocument()
	expect(screen.queryByText('Loading settings')).not.toBeInTheDocument()
})

test('opens the screens in the interface locale when the settings could not be read', async () => {
	rememberFormatLocale(undefined)
	rememberLocale('en-US')
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	renderGate()

	expect(await screen.findByText('09/30/2026')).toBeInTheDocument()
})

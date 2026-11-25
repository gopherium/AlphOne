// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberLocale } from '@gopherium/gottext'
import { resetLocale } from '@gopherium/gottext/testing'
import { createAuthQueryClient, sessionQueryKey } from '@gopherium/react-auth'
import { QueryClientProvider, focusManager } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, onTestFinished, test } from 'vitest'

import {
	FormatLocaleGate,
	GraphProvider,
	createGraphClient,
	formatDate,
	rememberFormatLocale,
	useAdminSettings,
} from '../index'
import { HttpResponse, adminSession, delay, graphql, seedSession, server } from '../testing'

afterEach(() => {
	rememberFormatLocale(undefined)
	resetLocale()
})

/** How long a test waits for a read the screens must not ask again. */
const QUIET = 100

/** Shows one date as the screens write it. */
function Dated() {
	return <p>{formatDate(new Date(2026, 8, 30))}</p>
}

/** Shows the page sizes the admin settings serve, unknown while none are served. */
function SizedScreen() {
	const { settings } = useAdminSettings()
	return <p>{`Page sizes ${settings?.listPageSizes?.join(' ') ?? 'unknown'}`}</p>
}

/**
 * Renders the gate around a screen, signed in, the loading text standing in until it opens.
 * @param client - The query client the gate reads the settings through.
 * @param child - The screen the gate opens on.
 */
function renderGate(
	client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } }),
	child: ReactNode = <Dated />,
) {
	client.setQueryDefaults(sessionQueryKey, { staleTime: Infinity })
	seedSession(client, adminSession)
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>
				<FormatLocaleGate loading={<p>Loading settings</p>}>{child}</FormatLocaleGate>
			</GraphProvider>
		</QueryClientProvider>,
	)
}

/**
 * Answers every read of the admin settings with a failure, counting each read.
 * @returns The count of reads so far.
 */
function failSettings(): () => number {
	let asked = 0
	server.use(
		graphql.query('AdminSettings', () => {
			asked += 1
			return HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })
		}),
	)
	return () => asked
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
						toastActionMilliseconds: 10000,
						dialogWriteTimeoutMilliseconds: 60000,
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

test('opens the screens on the first failed read through the client the admin builds', async () => {
	rememberFormatLocale(undefined)
	rememberLocale('en-US')
	const asked = failSettings()

	renderGate(createAuthQueryClient())

	expect(await screen.findByText('09/30/2026')).toBeInTheDocument()
	expect(asked()).toBe(1)
})

test('keeps the screens open and asks once when a screen reads the settings a failed read left', async () => {
	rememberLocale('en-US')
	const asked = failSettings()

	renderGate(createAuthQueryClient(), <SizedScreen />)

	expect(await screen.findByText('Page sizes unknown')).toBeInTheDocument()
	await delay(QUIET)

	expect(screen.getByText('Page sizes unknown')).toBeInTheDocument()
	expect(screen.queryByText('Loading settings')).not.toBeInTheDocument()
	expect(asked()).toBe(1)
})

test('asks nothing again on window focus after a failed read', async () => {
	rememberFormatLocale(undefined)
	rememberLocale('en-US')
	const asked = failSettings()
	onTestFinished(() => focusManager.setFocused(undefined))

	renderGate(createAuthQueryClient())
	expect(await screen.findByText('09/30/2026')).toBeInTheDocument()
	act(() => {
		focusManager.setFocused(false)
		focusManager.setFocused(true)
	})
	await delay(QUIET)

	expect(screen.getByText('09/30/2026')).toBeInTheDocument()
	expect(asked()).toBe(1)
})

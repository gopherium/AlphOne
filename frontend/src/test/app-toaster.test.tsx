// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, GraphProvider, createGraphClient, useAdminSettings, useToaster } from '@alphone/frontend-sdk'
import { HttpResponse, adminSession, graphql, seedSession, server } from '@alphone/frontend-sdk/testing'
import { createAuthQueryClient, sessionQueryKey } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitForElementToBeRemoved } from '@testing-library/react'
import { resetLocaleData, setLocaleData } from '@wordpress/i18n'
import { afterEach, expect, test } from 'vitest'

import { AppToaster, PublicToaster } from '../AppToaster'

afterEach(() => {
	resetLocaleData(undefined, 'alphone')
})

/** Raises one toast carrying an Undo when its button is pressed. */
function Raiser() {
	const toaster = useToaster()
	return <Button onClick={() => toaster.show('Task completed.', { label: 'Undo', onAct: () => {} })}>Complete</Button>
}

/** Raises one toast with no action when its button is pressed. */
function PlainRaiser() {
	const toaster = useToaster()
	return <Button onClick={() => toaster.show('Contact saved.')}>Save</Button>
}

/** Offers the given raise button once the admin settings arrived. */
function Settled({ children }: { children: React.ReactNode }) {
	return useAdminSettings().settings === undefined ? null : children
}

/**
 * Answers the admin settings with the given toast times.
 * @param toastMilliseconds - How long a toast with no button stays.
 * @param toastActionMilliseconds - How long a toast with a button stays.
 */
function serveToastTimes(toastMilliseconds: number, toastActionMilliseconds: number) {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds,
						toastActionMilliseconds,
						listPageSizes: [10, 20],
						listPageSize: 20,
						contactPageCap: 200,
						formatLocale: 'es-ES',
					},
				},
			}),
		),
	)
}

/**
 * Renders the app toaster around the given tree, signed in as the given account.
 * @param tree - The tree the toaster wraps.
 * @param user - The signed-in account, or null for nobody.
 */
function renderToaster(tree: React.ReactNode, user: (User & { role?: string }) | null) {
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, user)
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>
				<AppToaster>{tree}</AppToaster>
			</GraphProvider>
		</QueryClientProvider>,
	)
}

test('describes a plain toast with its hint in the language the page shows', async () => {
	setLocaleData({ 'Dismiss this notice': ['Descartar este aviso'] }, 'alphone')

	renderToaster(<PlainRaiser />, null)
	fireEvent.click(screen.getByRole('button', { name: 'Save' }))

	const toast = await screen.findByRole('button', { name: 'Contact saved.' })
	expect(toast).toHaveAccessibleDescription('Descartar este aviso')
})

test('keeps a plain toast on screen as long as the admin settings say', async () => {
	serveToastTimes(300, 60000)
	renderToaster(<Settled><PlainRaiser /></Settled>, adminSession)
	fireEvent.click(await screen.findByRole('button', { name: 'Save' }))

	await waitForElementToBeRemoved(() => screen.queryByText('Contact saved.'), { timeout: 2000 })
})

test('keeps a toast with a button on screen as long as the admin settings say', async () => {
	serveToastTimes(60000, 300)
	renderToaster(<Settled><Raiser /></Settled>, adminSession)
	fireEvent.click(await screen.findByRole('button', { name: 'Complete' }))

	await waitForElementToBeRemoved(() => screen.queryByText('Task completed.'), { timeout: 2000 })
})

test('raises toasts around a public link screen without asking who is signed in', async () => {
	const client = createAuthQueryClient({ queries: { retry: false } })
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>
				<PublicToaster>
					<Raiser />
				</PublicToaster>
			</GraphProvider>
		</QueryClientProvider>,
	)
	fireEvent.click(screen.getByRole('button', { name: 'Complete' }))

	expect(await screen.findByText('Task completed.')).toBeInTheDocument()
	expect(client.getQueryCache().find({ queryKey: sessionQueryKey })).toBeUndefined()
})

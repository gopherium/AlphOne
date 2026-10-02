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

/** Offers the raise button once the admin settings arrived. */
function SettledRaiser() {
	return useAdminSettings().settings === undefined ? null : <Raiser />
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

test('names the control clearing a toast in the language the page shows', async () => {
	setLocaleData({ Dismiss: ['Descartar'] }, 'alphone')

	renderToaster(<Raiser />, null)
	fireEvent.click(screen.getByRole('button', { name: 'Complete' }))

	expect(await screen.findByText('Task completed.')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Descartar' })).toBeInTheDocument()
})

test('keeps a toast on screen as long as the admin settings say', async () => {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds: 300,
						listPageSizes: [10, 20],
						listPageSize: 20,
						contactPageCap: 200,
						formatLocale: 'es-ES',
					},
				},
			}),
		),
	)
	renderToaster(<SettledRaiser />, adminSession)
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

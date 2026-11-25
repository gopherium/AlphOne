// SPDX-License-Identifier: AGPL-3.0-or-later

import { AccountGraph, FormatLocaleGate, Text, __ } from '@alphone/frontend-sdk'
import type { GraphKeys } from '@alphone/frontend-sdk'
import { AuthGate } from '@gopherium/react-auth'
import type { AuthLink } from '@gopherium/react-auth/wpds'
import { QueryClientProvider } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import type { ReactNode } from 'react'

import { AdminHost } from './AdminHost'
import { AppToaster, PublicToaster } from './AppToaster'
import { LoginSlot } from './auth/PublicAuth'
import { BootLoading } from './boot'
import type { createAppRouter } from './router'

/** AppProps are what the admin renders from. */
interface AppProps {
	/** queryClient is the client every react-query read goes through. */
	queryClient: QueryClient
	/** router is the router of the admin screens. */
	router: ReturnType<typeof createAppRouter>
	/** links are the links under the account screens. */
	links: readonly AuthLink[]
	/** publicScreen is the screen a mailed account link asks for, or null for the admin. */
	publicScreen: ReactNode
	/** graphKeys are the keys the graph cache files each type without an id by. */
	graphKeys: GraphKeys
}

/**
 * Renders the admin behind its sign-in, or a public link screen in its place.
 * @param props - The query client, the router, the links, the public screen and the graph keys.
 * @returns The admin tree.
 */
export function App({ queryClient, router, links, publicScreen, graphKeys }: AppProps) {
	return (
		<QueryClientProvider client={queryClient}>
			<AdminHost>
				{publicScreen === null ? (
					<AuthGate
						loginScreen={(onLogin) => <LoginSlot links={links} onLogin={onLogin} />}
						loading={<BootLoading />}
						error={<Text role="alert">{__('Something went wrong.', 'alphone')}</Text>}
					>
						<AccountGraph keys={graphKeys}>
							<AppToaster>
								<FormatLocaleGate loading={<BootLoading />}>
									<RouterProvider router={router} />
								</FormatLocaleGate>
							</AppToaster>
						</AccountGraph>
					</AuthGate>
				) : (
					<PublicToaster>{publicScreen}</PublicToaster>
				)}
			</AdminHost>
		</QueryClientProvider>
	)
}

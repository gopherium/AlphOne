// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, Text, createGraphClient } from '@alphone/frontend-sdk'
import { AdminRoot } from '@gopherium/godmin'
import {
	AuthGate,
	configureAuthTransport,
	createAuthQueryClient,
	sessionQueryKey,
} from '@gopherium/react-auth'
import { LoginScreen } from '@gopherium/react-auth/wpds'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@gopherium/godmin/base.css'
import '@gopherium/react-auth/wpds/style.css'
import './index.css'
import { graphAuthTransport } from './auth/graphTransport'
import { createAppRouter } from './router'

configureAuthTransport(graphAuthTransport)
const queryClient = createAuthQueryClient()
const graph = createGraphClient({
	onSessionExpired: () => queryClient.setQueryData(sessionQueryKey, null),
})
const router = createAppRouter()

createRoot(document.getElementById('root')!).render(
	<StrictMode>
		<QueryClientProvider client={queryClient}>
			<GraphProvider graph={graph}>
				<AdminRoot>
					<AuthGate
						loginScreen={(onLogin) => (
							<LoginScreen brand="AlphOne" onLogin={onLogin} />
						)}
						loading={<Text role="status">Loading…</Text>}
						error={<Text role="alert">Something went wrong.</Text>}
					>
						<RouterProvider router={router} />
					</AuthGate>
				</AdminRoot>
			</GraphProvider>
		</QueryClientProvider>
	</StrictMode>,
)

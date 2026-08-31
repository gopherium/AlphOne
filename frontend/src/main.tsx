// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, Text, __, createGraphClient } from '@alphone/frontend-sdk'
import { AdminRoot } from '@gopherium/godmin'
import {
	AuthGate,
	configureAuthTransport,
	createAuthQueryClient,
	sessionQueryKey,
} from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@gopherium/godmin/base.css'
import '@gopherium/react-auth/wpds/style.css'
import './index.css'
import { graphAuthTransport } from './auth/graphTransport'
import { LoginSlot, publicAuthScreen } from './auth/PublicAuth'
import { BootLoading } from './boot'
import { configureAppErrorText } from './i18n/errors'
import { startAppLocale } from './i18n/start'
import { createAppRouter } from './router'

await startAppLocale()
configureAppErrorText()

configureAuthTransport(graphAuthTransport)
const queryClient = createAuthQueryClient()
const graph = createGraphClient({
	onSessionExpired: () => queryClient.setQueryData(sessionQueryKey, null),
})
const router = createAppRouter()
const publicScreen = publicAuthScreen(new URL(window.location.href), () => {
	window.location.assign('/')
})

createRoot(document.getElementById('root')!).render(
	<StrictMode>
		<QueryClientProvider client={queryClient}>
			<GraphProvider graph={graph}>
				<AdminRoot>
					{publicScreen ?? (
						<AuthGate
							loginScreen={(onLogin) => <LoginSlot onLogin={onLogin} />}
							loading={<BootLoading />}
							error={<Text role="alert">{__('Something went wrong.', 'alphone')}</Text>}
						>
							<RouterProvider router={router} />
						</AuthGate>
					)}
				</AdminRoot>
			</GraphProvider>
		</QueryClientProvider>
	</StrictMode>,
)

// SPDX-License-Identifier: AGPL-3.0-or-later

import { Text, ThemeProvider } from '@alphone/frontend-sdk'
import { AuthGate, createAuthQueryClient } from '@gopherium/react-auth'
import { LoginScreen } from '@gopherium/react-auth/wpds'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@wordpress/theme/design-tokens.css'
import '@gopherium/react-auth/wpds/style.css'
import './index.css'
import { createAppRouter } from './router'

const queryClient = createAuthQueryClient()
const router = createAppRouter()

createRoot(document.getElementById('root')!).render(
	<StrictMode>
		<QueryClientProvider client={queryClient}>
			<ThemeProvider isRoot>
				<AuthGate
					loginScreen={(onLogin) => (
						<LoginScreen brand="AlphOne" onLogin={onLogin} />
					)}
					loading={<Text role="status">Loading…</Text>}
					error={<Text role="alert">Something went wrong.</Text>}
				>
					<RouterProvider router={router} />
				</AuthGate>
			</ThemeProvider>
		</QueryClientProvider>
	</StrictMode>,
)

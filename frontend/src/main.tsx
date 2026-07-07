// SPDX-License-Identifier: AGPL-3.0-or-later

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@wordpress/theme/design-tokens.css'
import './index.css'
import { Inbox } from './inbox/Inbox'
import { ThemeProvider } from './ui'

const queryClient = new QueryClient()

createRoot(document.getElementById('root')!).render(
	<StrictMode>
		<QueryClientProvider client={queryClient}>
			<ThemeProvider isRoot>
				<Inbox />
			</ThemeProvider>
		</QueryClientProvider>
	</StrictMode>,
)

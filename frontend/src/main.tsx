// SPDX-License-Identifier: AGPL-3.0-or-later

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@wordpress/theme/design-tokens.css'
import './index.css'
import { createAppRouter } from './router'
import { ThemeProvider } from './ui'

const queryClient = new QueryClient()
const router = createAppRouter()

createRoot(document.getElementById('root')!).render(
	<StrictMode>
		<QueryClientProvider client={queryClient}>
			<ThemeProvider isRoot>
				<RouterProvider router={router} />
			</ThemeProvider>
		</QueryClientProvider>
	</StrictMode>,
)

// SPDX-License-Identifier: AGPL-3.0-or-later

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { render } from '@testing-library/react'

import { createAppRouter } from '../router'

export function renderAt(path: string) {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	})
	const router = createAppRouter(
		createMemoryHistory({ initialEntries: [path] }),
	)
	render(
		<QueryClientProvider client={client}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	)
}

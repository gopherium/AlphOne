// SPDX-License-Identifier: AGPL-3.0-or-later

import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { render } from '@testing-library/react'

import type { User } from '../auth/api'
import { sessionQueryKey } from '../auth/session'
import { createQueryClient } from '../queryClient'
import { createAppRouter } from '../router'

const defaultUser: User = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'grace@example.com',
	name: 'Grace Hopper',
}

export function renderAt(path: string, user: User | null = defaultUser) {
	const client = createQueryClient({
		queries: { retry: false, staleTime: Infinity },
	})
	client.setQueryData(sessionQueryKey, user)
	const router = createAppRouter(
		createMemoryHistory({ initialEntries: [path] }),
	)
	render(
		<QueryClientProvider client={client}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	)
	return client
}

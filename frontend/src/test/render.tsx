// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, createGraphClient } from '@alphone/frontend-sdk'
import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { createAuthQueryClient, sessionQueryKey } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { defaultUser, seedSession } from '@gopherium/react-auth/testing'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { render } from '@testing-library/react'

import { createAppRouter } from '../router'

export function renderAt(
	path: string,
	user: User | null = defaultUser,
	version: string | null = '0.1.0',
) {
	const client = createAuthQueryClient({
		queries: { retry: false, staleTime: Infinity },
	})
	seedSession(client, user)
	const graph = createGraphClient({
		onSessionExpired: () => client.setQueryData(sessionQueryKey, null),
	})
	server.use(
		graphql.query('Version', () =>
			version === null
				? HttpResponse.json({ data: null, errors: [{ message: 'the version is unavailable' }] })
				: HttpResponse.json({ data: { version } }),
		),
	)
	const router = createAppRouter(
		createMemoryHistory({ initialEntries: [path] }),
	)
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<RouterProvider router={router} />
			</GraphProvider>
		</QueryClientProvider>,
	)
	return client
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import type { DefaultOptions } from '@tanstack/react-query'

import { UnauthorizedError } from './auth/api'
import { sessionQueryKey } from './auth/session'

/**
 * Builds the app query client, dropping the cached session whenever a query
 * or mutation fails with an UnauthorizedError so the login screen takes over.
 * @param defaultOptions - Query and mutation defaults, mainly for tests.
 * @returns The configured query client.
 */
export function createQueryClient(defaultOptions?: DefaultOptions): QueryClient {
	const queryClient: QueryClient = new QueryClient({
		defaultOptions,
		queryCache: new QueryCache({ onError: dropExpiredSession }),
		mutationCache: new MutationCache({ onError: dropExpiredSession }),
	})

	/**
	 * Clears the cached session when the backend reports it gone.
	 * @param error - The failure reported by a query or mutation.
	 */
	function dropExpiredSession(error: unknown) {
		if (error instanceof UnauthorizedError) {
			queryClient.setQueryData(sessionQueryKey, null)
		}
	}

	return queryClient
}

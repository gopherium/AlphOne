// SPDX-License-Identifier: AGPL-3.0-or-later

import { hashKey, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { fetchSession, logout } from './api'

export const sessionQueryKey = ['session'] as const

/**
 * Loads the current session as a react-query result.
 * @returns The session query, whose data is the user or null.
 */
export function useSession() {
	return useQuery({
		queryKey: sessionQueryKey,
		queryFn: ({ signal }) => fetchSession(signal),
	})
}

/**
 * Ends the current session and drops all cached data belonging to the
 * signed-out user.
 * @returns The logout mutation.
 */
export function useLogout() {
	const queryClient = useQueryClient()
	return useMutation({
		mutationFn: logout,
		onSuccess: async () => {
			await queryClient.cancelQueries()
			queryClient.setQueryData(sessionQueryKey, null)
			queryClient.removeQueries({
				predicate: (query) => query.queryHash !== hashKey(sessionQueryKey),
			})
		},
	})
}

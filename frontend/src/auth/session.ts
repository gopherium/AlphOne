// SPDX-License-Identifier: AGPL-3.0-or-later

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

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
 * Ends the current session, dropping every cached query so no data from
 * the signed-out user survives for the next account on this browser.
 * @returns The logout mutation.
 */
export function useLogout() {
	const queryClient = useQueryClient()
	return useMutation({
		mutationFn: logout,
		onSuccess: async () => {
			await queryClient.cancelQueries()
			queryClient.removeQueries()
			queryClient.setQueryData(sessionQueryKey, null)
		},
	})
}

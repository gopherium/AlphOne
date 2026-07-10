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
		queryFn: fetchSession,
	})
}

/**
 * Ends the current session and clears the cached user.
 * @returns The logout mutation.
 */
export function useLogout() {
	const queryClient = useQueryClient()
	return useMutation({
		mutationFn: logout,
		onSuccess: () => queryClient.setQueryData(sessionQueryKey, null),
	})
}

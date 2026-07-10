// SPDX-License-Identifier: AGPL-3.0-or-later

import { Text } from '@alphone/frontend-sdk'
import { useQueryClient } from '@tanstack/react-query'
import type { ReactNode } from 'react'

import { LoginScreen } from './LoginScreen'
import { sessionQueryKey, useSession } from './session'

/**
 * Guards its children behind a login session, showing the login screen
 * until the user is authenticated.
 * @param children - The application to reveal once a session is active.
 * @returns The children, the login screen, or a status message.
 */
export function AuthGate({ children }: { children: ReactNode }) {
	const queryClient = useQueryClient()
	const session = useSession()

	if (session.isPending) {
		return <Text role="status">Loading…</Text>
	}
	if (session.isError) {
		return <Text role="alert">Something went wrong.</Text>
	}
	if (session.data === null) {
		return (
			<LoginScreen
				onLogin={(user) => queryClient.setQueryData(sessionQueryKey, user)}
			/>
		)
	}
	return <>{children}</>
}

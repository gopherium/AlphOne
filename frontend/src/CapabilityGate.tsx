// SPDX-License-Identifier: AGPL-3.0-or-later

import { can, useSession } from '@alphone/frontend-sdk'
import { Navigate, Outlet, useMatches } from '@tanstack/react-router'

/**
 * Renders the nested screen when the session holds every capability the routes declare, else sends the reader home.
 * @returns The nested screen, or the redirect standing in for it.
 */
export function CapabilityGate() {
	const session = useSession()
	const asked = useMatches().flatMap((match) => match.staticData.capability ?? [])
	if (asked.length === 0 || !asked.every((capability) => can(session, capability))) {
		return <Navigate to="/" replace />
	}
	return <Outlet />
}

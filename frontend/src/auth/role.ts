// SPDX-License-Identifier: AGPL-3.0-or-later

import { useSession } from '@gopherium/react-auth'

/**
 * Role is the tier a user stands in, where a scope narrows what a token carries.
 */
export type Role = 'admin' | 'member'

/**
 * Reads the tier the stored text names, member for anything it cannot read.
 * @param stored - The tier as the graph answers it.
 * @returns The tier, member when the text names no known tier.
 */
export function roleOf(stored: string | undefined): Role {
	return stored === 'admin' ? 'admin' : 'member'
}

/**
 * Reads the tier the signed-in account stands in.
 * @returns The caller's tier, member until the session says otherwise.
 */
export function useRole(): Role {
	const session = useSession().data as { role?: string } | null | undefined
	return roleOf(session?.role)
}

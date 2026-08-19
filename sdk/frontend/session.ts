// SPDX-License-Identifier: AGPL-3.0-or-later

import type { User } from '@gopherium/react-auth'
import { useSession as useAuthSession } from '@gopherium/react-auth'
import { useMemo } from 'react'

/** Role is the tier a signed-in account stands in. */
export type Role = 'admin' | 'member'

/** Session is the signed-in account as a plugin screen reads it. */
export interface Session {
	/** id is the account identifier. */
	id: string
	/** email is the account email address. */
	email: string
	/** name is the account display name. */
	name: string
	/** role is the tier the account stands in. */
	role: Role
}

/**
 * Reads the tier the stored text names, member for anything it cannot read.
 * @param stored - The tier as the graph answers it.
 * @returns The tier, member when the text names no known tier.
 */
export function roleOf(stored: string | undefined): Role {
	return stored === 'admin' ? 'admin' : 'member'
}

/**
 * Reads the signed-in account, null until a session is active.
 * @returns The session, or null without one.
 */
export function useSession(): Session | null {
	const account = useAuthSession().data as (User & { role?: string }) | null | undefined
	return useMemo(
		() =>
			account
				? { id: account.id, email: account.email, name: account.name, role: roleOf(account.role) }
				: null,
		[account],
	)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import type { User } from '@gopherium/react-auth'
import { useSession as useAuthSession } from '@gopherium/react-auth'
import { useMemo } from 'react'

/** Role is the role a signed-in account holds, as the deployment names it. */
export type Role = string

/** Capability is a named permission a screen or control asks for. */
export type Capability = string

/** The capability administering accounts. */
export const MANAGE_USERS: Capability = 'manage_users'

/** Session is the signed-in account as a plugin screen reads it. */
export interface Session {
	/** id is the account identifier. */
	id: string
	/** email is the account email address. */
	email: string
	/** name is the account display name. */
	name: string
	/** role is the role the account holds. */
	role: Role
	/** capabilities names what the account may do, as the server answered it. */
	capabilities: Capability[]
	/** grantable names the roles the account may give another account. */
	grantable: Role[]
}

/**
 * Reads the signed-in account, null until a session is active.
 * @returns The session, or null without one.
 */
export function useSession(): Session | null {
	const account = useAuthSession().data as
		| (User & { role?: string, capabilities?: string[], grantable?: string[] })
		| null
		| undefined
	return useMemo(
		() =>
			account
				? {
						id: account.id,
						email: account.email,
						name: account.name,
						role: account.role ?? '',
						capabilities: account.capabilities ?? [],
						grantable: account.grantable ?? [],
					}
				: null,
		[account],
	)
}

/**
 * Reports whether the session holds the capability, no session holding none.
 * @param session - The signed-in account, or nothing.
 * @param capability - The capability the decision point asks for.
 * @returns Whether the session holds it.
 */
export function can(
	session: { capabilities?: Capability[] } | null | undefined,
	capability: Capability,
): boolean {
	return (session?.capabilities ?? []).includes(capability)
}

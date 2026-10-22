// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x } from '@alphone/frontend-sdk'
import { RoleBeyondReachError, RoleUnknownError } from '@gopherium/react-auth/admin'

/** The error code each turned down role write answers with, by the error it raises. */
const refusals: [new (...args: never[]) => Error, string][] = [
	[RoleBeyondReachError, 'role_beyond_reach'],
	[RoleUnknownError, 'role_unknown'],
]

/**
 * Returns the label each core role reads as, read fresh so the loaded catalogue answers.
 * @returns The labels, keyed by the role the server stores.
 */
function coreRoleLabels(): Map<string, string> {
	return new Map<string, string>([
		['admin', _x('Administrator', 'role', 'alphone')],
		['member', _x('Member', 'role', 'alphone')],
		['', _x('No role', 'role', 'alphone')],
	])
}

/**
 * Returns the label a role reads as, a role a plugin adds reading as its stored name with a capital first letter.
 * @param role - The role the account holds.
 * @returns The label.
 */
export function roleLabel(role: string): string {
	return coreRoleLabels().get(role) ?? role.charAt(0).toUpperCase() + role.slice(1)
}

/** memberRole is the core role carrying no capability. */
const memberRole = 'member'

/**
 * Returns the role a new account starts under, member when the reader may give it, else the narrowest it may give.
 * @param grantable - The roles the reader may give, widest first.
 * @returns The starting role.
 */
export function startingRole(grantable: string[]): string {
	return grantable.includes(memberRole) ? memberRole : (grantable.at(-1) ?? memberRole)
}

/**
 * Returns the code a turned down role write carries, none for any other failure.
 * @param error - The failure the write raised.
 * @returns The error code, or nothing.
 */
export function roleRefusal(error: unknown): string | undefined {
	return refusals.find(([refused]) => error instanceof refused)?.[1]
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x } from '@alphone/frontend-sdk'
import type { RoleLabels } from '@gopherium/react-auth/wpds/users'

/**
 * Returns the label each core role reads as, read fresh so the loaded catalogue answers.
 * @returns The labels, keyed by the role the server stores.
 */
export function roleLabels(): RoleLabels {
	return {
		admin: _x('Administrator', 'role', 'alphone'),
		member: _x('Member', 'role', 'alphone'),
	}
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

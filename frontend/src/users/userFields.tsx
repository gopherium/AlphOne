// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, InitialsAvatar, __, _x, formatDate } from '@alphone/frontend-sdk'
import type { Field } from '@alphone/frontend-sdk/dataviews'

import type { Account } from '../auth/graphTransport'

/** UserStatus is the state an account stands in. */
type UserStatus = 'active' | 'invited' | 'disabled'

/** statuses lists every state an account stands in, in the order the filter offers them. */
const statuses: readonly UserStatus[] = ['active', 'invited', 'disabled']

/** statusIntents names the badge intent each state is drawn with, disabled in the outline one. */
const statusIntents: Record<UserStatus, 'stable' | 'informational' | 'none'> = {
	active: 'stable',
	invited: 'informational',
	disabled: 'none',
}

/**
 * Returns the state one account stands in.
 * @param user - The account.
 * @returns Disabled before invited, invited before active.
 */
function statusOf(user: Account): UserStatus {
	if (user.disabled) {
		return 'disabled'
	}
	if (!user.confirmed) {
		return 'invited'
	}
	return 'active'
}

/**
 * Returns the label each state reads as, read fresh so the loaded catalogue answers.
 * @returns The labels, keyed by state.
 */
function statusLabels(): Record<UserStatus, string> {
	return {
		active: _x('Active', 'user status', 'alphone'),
		invited: _x('Invited', 'user status', 'alphone'),
		disabled: _x('Disabled', 'user status', 'alphone'),
	}
}

/**
 * Returns the label a role reads as, falling back to what the server stored.
 * @param role - The role the account holds.
 * @returns The label.
 */
export function roleLabel(role: string): string {
	if (role === '') {
		return __('No role', 'alphone')
	}
	return role.charAt(0).toUpperCase() + role.slice(1)
}

/**
 * Builds the fields the users list shows, offering as roles only the ones the accounts hold.
 * @param users - Every account the list holds.
 * @returns The fields.
 */
export function userFields(users: readonly Account[]): Field<Account>[] {
	const labels = statusLabels()
	const roles = [...new Set(users.map((user) => user.role))]
		.map((role) => ({ value: role, label: roleLabel(role) }))
		.sort((a, b) => a.label.localeCompare(b.label))
	return [
		{
			id: 'name',
			label: __('Name', 'alphone'),
			type: 'text',
			enableGlobalSearch: true,
			enableHiding: false,
			filterBy: false,
		},
		{ id: 'email', label: __('Email', 'alphone'), type: 'text', enableGlobalSearch: true, filterBy: false },
		{
			id: 'avatar',
			label: __('Avatar', 'alphone'),
			render: ({ item }) => <InitialsAvatar name={item.name} />,
			enableSorting: false,
			filterBy: false,
		},
		{
			id: 'status',
			label: __('Status', 'alphone'),
			type: 'text',
			getValue: ({ item }) => statusOf(item),
			elements: statuses.map((status) => ({ value: status, label: labels[status] })),
			filterBy: { operators: ['isAny'] },
			render: ({ item }) => <Badge intent={statusIntents[statusOf(item)]}>{labels[statusOf(item)]}</Badge>,
		},
		{
			id: 'role',
			label: __('Role', 'alphone'),
			type: 'text',
			elements: roles,
			filterBy: { operators: ['isAny'] },
			render: ({ item }) => roleLabel(item.role),
		},
		{
			id: 'created',
			label: __('Created', 'alphone'),
			getValue: ({ item }) => item.created_at.getTime(),
			render: ({ item }) => formatDate(item.created_at),
			filterBy: false,
		},
	]
}

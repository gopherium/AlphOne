// SPDX-License-Identifier: AGPL-3.0-or-later

import { SelectControl, __ } from '@alphone/frontend-sdk'
import type { Role } from '@alphone/frontend-sdk'

import { roleLabel } from './roles'

/**
 * Renders the select offering the roles the reader may give.
 * @param props - The roles the reader may give, the role chosen, and the handler taking another choice.
 * @returns The role select.
 */
export function RoleSelect({
	grantable,
	value,
	onChange,
}: {
	grantable: Role[]
	value: Role
	onChange: (role: Role) => void
}) {
	const offered = grantable.map((role) => ({ value: role, label: roleLabel(role) }))
	return (
		<SelectControl
			label={__('Role', 'alphone')}
			items={offered}
			value={offered.find((option) => option.value === value)}
			onValueChange={(item) => item?.value != null && onChange(item.value)}
		/>
	)
}

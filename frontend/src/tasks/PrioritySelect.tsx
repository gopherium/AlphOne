// SPDX-License-Identifier: AGPL-3.0-or-later

import { SelectControl } from '@alphone/frontend-sdk'

import { priorityOf } from './priority'

const priorities = [
	{ value: '0', label: 'Normal' },
	{ value: '1', label: 'High' },
]

/**
 * Renders the priority chooser for a task.
 * @returns The priority select.
 */
export function PrioritySelect({
	value,
	onChange,
}: {
	value: number
	onChange: (priority: number) => void
}) {
	return (
		<SelectControl
			label="Priority"
			items={priorities}
			value={priorities[value] ?? priorities[0]}
			onValueChange={(item) => onChange(priorityOf(item))}
		/>
	)
}

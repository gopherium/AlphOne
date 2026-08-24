// SPDX-License-Identifier: AGPL-3.0-or-later

import { SelectControl, _x, __ } from '@alphone/frontend-sdk'

import { priorityOf } from './priority'

/**
 * Returns the priority options, read fresh so the loaded catalogue answers.
 * @returns The options to offer.
 */
function priorities(): { value: string; label: string }[] {
	return [
		{ value: '0', label: _x('Normal', 'task priority', 'alphone') },
		{ value: '1', label: _x('High', 'task priority', 'alphone') },
	]
}

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
	const items = priorities()
	return (
		<SelectControl
			label={__('Priority', 'alphone')}
			items={items}
			value={items[value] ?? items[0]}
			onValueChange={(item) => onChange(priorityOf(item))}
		/>
	)
}

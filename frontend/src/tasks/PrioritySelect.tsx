// SPDX-License-Identifier: AGPL-3.0-or-later

const priorities = [
	{ value: 0, label: 'Normal' },
	{ value: 1, label: 'High' },
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
		<label className="alphone-tasks__field">
			Priority
			<select value={String(value)} onChange={(event) => onChange(Number(event.target.value))}>
				{priorities.map((priority) => (
					<option key={priority.value} value={priority.value}>
						{priority.label}
					</option>
				))}
			</select>
		</label>
	)
}

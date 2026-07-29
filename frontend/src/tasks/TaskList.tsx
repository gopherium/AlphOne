// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, Text } from '@alphone/frontend-sdk'
import type { InfiniteData, UseInfiniteQueryResult } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import type { Task, TaskPage } from './api'

export type TaskQuery = UseInfiniteQueryResult<InfiniteData<TaskPage>, Error>

export interface RowControls {
	onChange: (task: Task) => void
	onPush: (task: Task) => void
	pendingID: string
}

/**
 * Renders one labelled list of task rows.
 * @returns The task list.
 */
export function TaskList({
	label,
	tasks,
	controls,
	showDueDate = false,
}: {
	label: string
	tasks: Task[]
	controls: RowControls
	showDueDate?: boolean
}) {
	return (
		<ul className="alphone-tasks__list" aria-label={label}>
			{tasks.map((task) => (
				<li key={task.id} className="alphone-tasks__row" aria-label={task.title}>
					<Text className={task.status === 'done' ? 'alphone-tasks__title--done' : undefined}>
						<Link to="/tasks/$taskId" params={{ taskId: task.id }}>
							{task.title}
						</Link>
					</Text>
					{showDueDate ? (
						<Text className="alphone-tasks__due">{`Due ${task.due_on}`}</Text>
					) : null}
					{task.status === 'done' ? null : (
						<Button variant="minimal" onClick={() => controls.onPush(task)}>
							Push to tomorrow
						</Button>
					)}
					<Button
						variant="outline"
						disabled={controls.pendingID === task.id}
						onClick={() => controls.onChange(task)}
					>
						{task.status === 'done' ? 'Reopen' : 'Complete'}
					</Button>
				</li>
			))}
		</ul>
	)
}

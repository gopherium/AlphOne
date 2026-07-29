// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Button, IconButton, Link, Stack, Text, check } from '@alphone/frontend-sdk'
import type { InfiniteData, UseInfiniteQueryResult } from '@tanstack/react-query'
import { Link as RouterLink } from '@tanstack/react-router'

import type { Task, TaskPage } from './api'
import { formatDue } from './format'

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
		<Stack
			direction="column"
			className="alphone-tasks__list"
			aria-label={label}
			render={<ul />}
		>
			{tasks.map((task) => (
				<TaskRow
					key={task.id}
					task={task}
					controls={controls}
					showDueDate={showDueDate}
				/>
			))}
		</Stack>
	)
}

/**
 * Renders one task as a row with its completion and reschedule controls.
 * @returns The task row.
 */
function TaskRow({
	task,
	controls,
	showDueDate,
}: {
	task: Task
	controls: RowControls
	showDueDate: boolean
}) {
	const done = task.status === 'done'
	return (
		<Stack
			direction="row"
			gap="md"
			align="center"
			className="alphone-tasks__row"
			aria-label={task.title}
			render={<li />}
		>
			<IconButton
				icon={check}
				label={done ? 'Reopen' : 'Complete'}
				variant={done ? 'solid' : 'minimal'}
				tone={done ? 'brand' : 'neutral'}
				size="compact"
				className="alphone-tasks__check"
				disabled={controls.pendingID === task.id}
				onClick={() => controls.onChange(task)}
			/>
			<Text
				variant="body-md"
				className={`alphone-tasks__title${done ? ' alphone-tasks__title--done' : ''}`}
			>
				<Link
					variant="unstyled"
					render={<RouterLink to="/tasks/$taskId" params={{ taskId: task.id }} />}
				>
					{task.title}
				</Link>
			</Text>
			{task.priority > 0 ? <Badge intent="high">High</Badge> : null}
			{showDueDate ? (
				<Text variant="body-sm" className="alphone-tasks__due">
					{formatDue(task.due_on)}
				</Text>
			) : null}
			{done ? null : (
				<Button
					variant="minimal"
					tone="neutral"
					size="compact"
					className="alphone-tasks__push"
					onClick={() => controls.onPush(task)}
				>
					Push to tomorrow
				</Button>
			)}
		</Stack>
	)
}

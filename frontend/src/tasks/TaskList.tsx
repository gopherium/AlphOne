// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Button, Checkbox, Link, Stack, Text } from '@alphone/frontend-sdk'
import type { ConnectionResult } from '@alphone/frontend-sdk'
import { Link as RouterLink } from '@tanstack/react-router'

import { formatDue } from './format'

/** ListedTask is the part of a task every list row and its controls read. */
export interface ListedTask {
	id: string
	title: string
	status: string
	priority: number
	due_on: string
}

export interface RowControls {
	onChange: (task: ListedTask) => void
	onPush: (task: ListedTask) => void
	pendingID: string
}

/** GraphTask is one task as the graph documents select it. */
export interface GraphTask {
	id: string
	title: string
	status: string
	priority: number
	dueOn: string
}

/**
 * Maps a graph task connection onto the rows the task list renders.
 * @param connection - The task connection as the graph serves it.
 * @returns The connection carrying list shaped rows.
 */
export function toListedTasks(
	connection: ConnectionResult<GraphTask>,
): ConnectionResult<ListedTask> {
	return {
		...connection,
		rows: connection.rows.map((node) => ({
			id: node.id,
			title: node.title,
			status: node.status,
			priority: node.priority,
			due_on: node.dueOn,
		})),
	}
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
	tasks: readonly ListedTask[]
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
	task: ListedTask
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
			<Checkbox
				aria-label={done ? 'Reopen' : 'Complete'}
				checked={done}
				disabled={controls.pendingID === task.id}
				className="alphone-tasks__check"
				onCheckedChange={() => controls.onChange(task)}
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

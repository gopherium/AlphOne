// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
	Button,
	Checkbox,
	IconButton,
	Link,
	Stack,
	Text,
	__,
	_x,
	commentAuthorAvatar,
	sprintf,
} from '@alphone/frontend-sdk'
import { Link as RouterLink } from '@tanstack/react-router'

import { formatDue } from './format'

/** ListedTask is the part of a task every list row and its controls read. */
export interface ListedTask {
	id: string
	title: string
	status: string
	priority: number
	dueOn: string
	contact?: { id: string; name: string } | null
}

export interface RowControls {
	onChange: (task: ListedTask) => void
	onPush: (task: ListedTask) => void
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
				aria-label={done ? __('Reopen', 'alphone') : __('Complete', 'alphone')}
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
				{task.contact ? <OpenContact contact={task.contact} /> : null}
			</Text>
			{task.priority > 0 ? <Badge intent="high">{_x('High', 'task priority', 'alphone')}</Badge> : null}
			{showDueDate ? (
				<Text variant="body-sm" className="alphone-tasks__due">
					{formatDue(task.dueOn)}
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
					{__('Postpone', 'alphone')}
				</Button>
			)}
		</Stack>
	)
}

/**
 * Renders the person icon that opens the contact a task is linked to.
 * @returns The contact icon link.
 */
function OpenContact({ contact }: { contact: { id: string; name: string } }) {
	return (
		<IconButton
			icon={commentAuthorAvatar}
			label={sprintf(__('Open %(name)s', 'alphone'), { name: contact.name })}
			variant="minimal"
			tone="neutral"
			size="small"
			className="alphone-tasks__open-contact"
			nativeButton={false}
			role="link"
			render={<RouterLink to="/contacts/$contactId" params={{ contactId: contact.id }} />}
		/>
	)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
	Button,
	ButtonLink,
	Checkbox,
	Link,
	Stack,
	Text,
	__,
	_x,
	commentAuthorAvatar,
	formatTime,
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
	dueTime: string | null
	contact?: { id: string; name: string } | null
}

export interface RowControls {
	onChange: (task: ListedTask) => void
	onPush: (task: ListedTask) => void
	pendingID: string
}

/** ClockRoom holds the widest times the format locale writes, one each side of noon, sizing every time column. */
interface ClockRoom {
	morning: string
	evening: string
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
	const room = { morning: formatTime('10:00'), evening: formatTime('22:00') }
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
					room={room}
				/>
			))}
		</Stack>
	)
}

/**
 * Renders the time a task is due at in the column before its title, blank for a task with no time.
 * @returns The time column.
 */
function DueTime({ time, room }: { time: string | null; room: ClockRoom }) {
	const column = { className: 'alphone-tasks__time', 'data-morning': room.morning, 'data-evening': room.evening }
	if (time === null) {
		return <Text variant="body-md" {...column} />
	}
	return (
		<Text variant="body-md" {...column} render={<time dateTime={time} />}>
			{formatTime(time)}
		</Text>
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
	room,
}: {
	task: ListedTask
	controls: RowControls
	showDueDate: boolean
	room: ClockRoom
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
			<Stack direction="row" gap="md" align="start" className="alphone-tasks__lead">
				<DueTime time={task.dueTime} room={room} />
				<Text
					variant="body-md"
					className={`alphone-tasks__title${done ? ' alphone-tasks__title--done' : ''}`}
				>
					<Link
						variant="unstyled"
						className="alphone-tasks__title-link"
						render={<RouterLink to="/tasks/$taskId" params={{ taskId: task.id }} />}
					>
						{task.title}
					</Link>
					{task.contact ? <OpenContact contact={task.contact} /> : null}
				</Text>
			</Stack>
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
 * Renders the person icon and name that open the contact a task is linked to.
 * @returns The contact link.
 */
function OpenContact({ contact }: { contact: { id: string; name: string } }) {
	return (
		<ButtonLink
			variant="minimal"
			tone="neutral"
			size="small"
			className="alphone-tasks__open-contact"
			render={<RouterLink to="/contacts/$contactId" params={{ contactId: contact.id }} />}
		>
			<ButtonLink.Icon icon={commentAuthorAvatar} className="alphone-tasks__contact-icon" />
			<span className="alphone-tasks__contact-name">{contact.name}</span>
		</ButtonLink>
	)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	Collapsible,
	EmptyState,
	IconButton,
	InputControl,
	LoadingRows,
	LoadMore,
	ErrorNotice,
	PageScreen,
	Stack,
	Text,
	chevronLeft,
	chevronRight,
	inbox,
	validationMessage,
} from '@alphone/frontend-sdk'
import { graphError, useConnection, useGraph, useGraphMutation } from '@alphone/frontend-sdk'
import type { ConnectionResult } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'

import { formatDay, laterDate, shiftDate } from './format'
import { createTaskMutation, dayTasksQuery, overdueTasksQuery, updateTaskMutation } from './operations'
import { TaskList } from './TaskList'
import type { ListedTask, RowControls } from './TaskList'

const tasksPageSize = 50

/** taskOperations names the documents a task write must refresh. */
const taskOperations = ['DayTasks', 'OverdueTasks']

/**
 * Renders a day of tasks with its overdue work, quick add field, and done
 * group.
 * @returns The tasks screen.
 */
export function TasksScreen({ date, today }: { date: string; today: string }) {
	const graph = useGraph()
	const tasks = useConnection({
		query: dayTasksQuery,
		variables: { date, status: 'open', first: tasksPageSize },
		select: (data) => data.tasks,
	})
	const done = useConnection({
		query: dayTasksQuery,
		variables: { date, status: 'done', first: tasksPageSize },
		select: (data) => data.tasks,
	})
	const overdue = useConnection({
		query: overdueTasksQuery,
		variables: { dueBefore: today, status: 'open', first: tasksPageSize },
		select: (data) => data.tasks,
		pause: date !== today,
	})
	const [title, setTitle] = useState('')
	const [pendingID, setPendingID] = useState('')
	const [add, runAdd] = useGraphMutation(createTaskMutation)
	const [change, runChange] = useGraphMutation(updateTaskMutation)
	const [push, runPush] = useGraphMutation(updateTaskMutation)
	const submitAdd = async () => {
		const result = await runAdd({ input: { title, dueOn: date } })
		if (result.data) {
			setTitle('')
			graph.refetch(taskOperations)
		}
	}
	const changeStatus = async (task: ListedTask) => {
		setPendingID(task.id)
		const result = await runChange({
			id: task.id,
			input: { status: task.status === 'done' ? 'open' : 'done' },
		})
		setPendingID('')
		if (result.data) {
			graph.refetch(taskOperations)
		}
	}
	const pushTask = async (task: ListedTask) => {
		const result = await runPush({
			id: task.id,
			input: { dueOn: shiftDate(laterDate(task.dueOn, today), 1) },
		})
		if (result.data) {
			graph.refetch(taskOperations)
		}
	}
	const controls: RowControls = {
		onChange: (task) => void changeStatus(task),
		onPush: (task) => void pushTask(task),
		pendingID,
	}

	return (
		<PageScreen
			title="Tasks"
			subtitle={formatDay(date)}
			actions={
				<>
					<DayNavigation date={date} today={today} />
					<Button variant="solid" render={<Link to="/tasks/new" search={{ date }} />}>
						New task
					</Button>
				</>
			}
		>
			{date === today ? <OverdueSection tasks={overdue} controls={controls} /> : null}
			<form
				className="alphone-tasks__add"
				onSubmit={(event) => {
					event.preventDefault()
					void submitAdd()
				}}
			>
				<InputControl
					label="New task"
					hideLabelFromVision
					placeholder="Add a task"
					value={title}
					onChange={(event) => setTitle(event.target.value)}
				/>
				<Button
					type="submit"
					disabled={title.trim() === '' || add.fetching}
					loading={add.fetching}
				>
					Add task
				</Button>
			</form>
			{add.error ? (
				<ErrorNotice>
					{validationMessage(graphError(add.error), 'The task could not be added.')}
				</ErrorNotice>
			) : null}
			{change.error || push.error ? (
				<ErrorNotice>The task could not be updated.</ErrorNotice>
			) : null}
			<TaskSections tasks={tasks} done={done} controls={controls} />
		</PageScreen>
	)
}

/**
 * Renders the controls that move between days.
 * @returns The day navigation.
 */
function DayNavigation({ date, today }: { date: string; today: string }) {
	return (
		<Stack direction="row" gap="xs" align="center">
			<IconButton
				icon={chevronLeft}
				label="Previous day"
				variant="minimal"
				tone="neutral"
				nativeButton={false}
				render={<Link to="/tasks" search={{ date: shiftDate(date, -1) }} />}
			/>
			{date === today ? null : (
				<Button
					variant="minimal"
					tone="neutral"
					size="compact"
					nativeButton={false}
					render={<Link to="/tasks" search={{ date: today }} />}
				>
					Today
				</Button>
			)}
			<IconButton
				icon={chevronRight}
				label="Next day"
				variant="minimal"
				tone="neutral"
				nativeButton={false}
				render={<Link to="/tasks" search={{ date: shiftDate(date, 1) }} />}
			/>
		</Stack>
	)
}

/**
 * Renders the open tasks left over from earlier days.
 * @returns The overdue section.
 */
function OverdueSection({
	tasks,
	controls,
}: {
	tasks: ConnectionResult<ListedTask>
	controls: RowControls
}) {
	if (tasks.isError) {
		return (
			<ErrorNotice>Overdue tasks could not be loaded.</ErrorNotice>
		)
	}
	const rows = tasks.rows
	if (rows.length === 0) {
		return null
	}
	return (
		<Stack direction="column" gap="sm" className="alphone-tasks__overdue">
			<Text variant="heading-sm" render={<h2 />}>
				Overdue
			</Text>
			<TaskList label="Overdue tasks" tasks={rows} controls={controls} showDueDate />
			<LoadMore query={tasks}>Load more overdue</LoadMore>
		</Stack>
	)
}

/**
 * Renders the loading, error, and loaded states of the day's tasks.
 * @returns The task sections.
 */
function TaskSections({
	tasks,
	done,
	controls,
}: {
	tasks: ConnectionResult<ListedTask>
	done: ConnectionResult<ListedTask>
	controls: RowControls
}) {
	if (tasks.isPending) {
		return <LoadingRows label="Loading tasks…" />
	}
	if (tasks.isError) {
		return (
			<ErrorNotice>Tasks could not be loaded.</ErrorNotice>
		)
	}
	const open = tasks.rows
	return (
		<Stack direction="column" gap="lg" className="godmin-arrival">
			{open.length === 0 ? (
				<EmptyState.Root className="godmin-empty">
					<EmptyState.Icon icon={inbox} />
					<EmptyState.Title>Nothing due today.</EmptyState.Title>
					<EmptyState.Description>
						Add a task above, or open a contact and create one from their page.
					</EmptyState.Description>
				</EmptyState.Root>
			) : (
				<TaskList label="Open tasks" tasks={open} controls={controls} />
			)}
			<LoadMore query={tasks}>Load more</LoadMore>
			<DoneGroup tasks={done} controls={controls} />
		</Stack>
	)
}

/**
 * Renders the collapsible group of tasks completed on the day.
 * @returns The done group.
 */
function DoneGroup({
	tasks,
	controls,
}: {
	tasks: ConnectionResult<ListedTask>
	controls: RowControls
}) {
	if (tasks.isError) {
		return null
	}
	const rows = tasks.rows
	if (rows.length === 0) {
		return null
	}
	return (
		<Collapsible.Root>
			<Collapsible.Trigger
				render={
					<Button variant="minimal" tone="neutral" size="compact">
						{`Done (${rows.length}${tasks.hasNextPage ? '+' : ''})`}
					</Button>
				}
			/>
			<Collapsible.Panel>
				<Stack direction="column" gap="sm">
					<TaskList label="Done tasks" tasks={rows} controls={controls} />
					<LoadMore query={tasks}>Load more done</LoadMore>
				</Stack>
			</Collapsible.Panel>
		</Collapsible.Root>
	)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, InputControl, Text } from '@alphone/frontend-sdk'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type { InfiniteData, UseInfiniteQueryResult } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'

import {
	ValidationError,
	createTask,
	fetchDayTasks,
	fetchOverdueTasks,
	patchTask,
} from './api'
import type { Task, TaskPage } from './api'
import { formatDay, laterDate, shiftDate } from './format'

type TaskQuery = UseInfiniteQueryResult<InfiniteData<TaskPage>, Error>

/**
 * Copy for a failed create.
 * @param error - The mutation error.
 * @returns The message shown under the quick add field.
 */
function addErrorText(error: unknown): string {
	if (error instanceof ValidationError) {
		return error.message
	}
	return 'The task could not be added.'
}

/**
 * Renders a day of tasks with its overdue work, quick add field, and done
 * group.
 * @returns The tasks screen.
 */
export function TasksScreen({ date, today }: { date: string; today: string }) {
	const queryClient = useQueryClient()
	const tasks = useInfiniteQuery({
		queryKey: ['tasks', 'day', date],
		queryFn: ({ pageParam }) => fetchDayTasks(date, pageParam),
		initialPageParam: '',
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
	})
	const overdue = useInfiniteQuery({
		queryKey: ['tasks', 'overdue', today],
		queryFn: ({ pageParam }) => fetchOverdueTasks(today, pageParam),
		initialPageParam: '',
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		enabled: date === today,
	})
	const [title, setTitle] = useState('')
	const add = useMutation({
		mutationFn: () => createTask(title, date),
		onSuccess: async () => {
			setTitle('')
			await queryClient.invalidateQueries({ queryKey: ['tasks'] })
		},
	})
	const change = useMutation({
		mutationFn: (task: Task) =>
			patchTask(task.id, { status: task.status === 'done' ? 'open' : 'done' }),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
	})
	const push = useMutation({
		mutationFn: (task: Task) =>
			patchTask(task.id, { due_on: shiftDate(laterDate(task.due_on, today), 1) }),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
	})
	const controls: RowControls = {
		onChange: (task) => change.mutate(task),
		onPush: (task) => push.mutate(task),
		pendingID: change.isPending ? change.variables.id : '',
	}

	return (
		<div className="alphone-tasks">
			<div className="alphone-tasks__header">
				<h1>Tasks</h1>
				<DayNavigation date={date} today={today} />
			</div>
			<Text className="alphone-tasks__day">{formatDay(date)}</Text>
			{date === today ? <OverdueSection tasks={overdue} controls={controls} /> : null}
			<form
				className="alphone-tasks__add"
				onSubmit={(event) => {
					event.preventDefault()
					add.mutate()
				}}
			>
				<InputControl
					label="New task"
					hideLabelFromVision
					placeholder="Add a task"
					value={title}
					onChange={(event) => setTitle(event.target.value)}
				/>
				<Button type="submit" disabled={title.trim() === '' || add.isPending}>
					Add task
				</Button>
			</form>
			{add.isError ? <Text role="alert">{addErrorText(add.error)}</Text> : null}
			{change.isError || push.isError ? (
				<Text role="alert">The task could not be updated.</Text>
			) : null}
			<TaskSections tasks={tasks} controls={controls} />
		</div>
	)
}

/**
 * Renders the links that move between days.
 * @returns The day navigation.
 */
function DayNavigation({ date, today }: { date: string; today: string }) {
	return (
		<div className="alphone-tasks__nav">
			<Link to="/tasks" search={{ date: shiftDate(date, -1) }} aria-label="Previous day">
				‹
			</Link>
			{date === today ? null : (
				<Link to="/tasks" search={{ date: today }}>
					Today
				</Link>
			)}
			<Link to="/tasks" search={{ date: shiftDate(date, 1) }} aria-label="Next day">
				›
			</Link>
		</div>
	)
}

/**
 * Renders the open tasks left over from earlier days.
 * @returns The overdue section.
 */
function OverdueSection({ tasks, controls }: { tasks: TaskQuery; controls: RowControls }) {
	if (tasks.isPending) {
		return null
	}
	if (tasks.isError) {
		return <Text role="alert">Overdue tasks could not be loaded.</Text>
	}
	const rows = tasks.data.pages.flatMap((page) => page.tasks)
	if (rows.length === 0) {
		return null
	}
	return (
		<div className="alphone-tasks__overdue">
			<h2>Overdue</h2>
			<TaskList label="Overdue tasks" tasks={rows} controls={controls} showDueDate />
			{tasks.hasNextPage ? (
				<Button onClick={() => void tasks.fetchNextPage()} disabled={tasks.isFetchingNextPage}>
					Load more overdue
				</Button>
			) : null}
		</div>
	)
}

/**
 * Renders the loading, error, and loaded states of the day's tasks.
 * @returns The task sections.
 */
function TaskSections({ tasks, controls }: { tasks: TaskQuery; controls: RowControls }) {
	if (tasks.isPending) {
		return <Text role="status">Loading tasks…</Text>
	}
	if (tasks.isError) {
		return <Text role="alert">Tasks could not be loaded.</Text>
	}
	const rows = tasks.data.pages.flatMap((page) => page.tasks)
	const open = rows.filter((task) => task.status !== 'done')
	const done = rows.filter((task) => task.status === 'done')
	return (
		<>
			{open.length === 0 ? (
				<Text role="status">Nothing due today.</Text>
			) : (
				<TaskList label="Open tasks" tasks={open} controls={controls} />
			)}
			{tasks.hasNextPage ? (
				<Button onClick={() => void tasks.fetchNextPage()} disabled={tasks.isFetchingNextPage}>
					Load more
				</Button>
			) : null}
			{done.length > 0 ? <DoneGroup tasks={done} controls={controls} /> : null}
		</>
	)
}

/**
 * Renders the collapsible group of tasks completed on the day.
 * @returns The done group.
 */
function DoneGroup({ tasks, controls }: { tasks: Task[]; controls: RowControls }) {
	const [open, setOpen] = useState(false)
	return (
		<div className="alphone-tasks__done">
			<Button variant="minimal" onClick={() => setOpen(!open)}>
				{`Done (${tasks.length})`}
			</Button>
			{open ? <TaskList label="Done tasks" tasks={tasks} controls={controls} /> : null}
		</div>
	)
}

interface RowControls {
	onChange: (task: Task) => void
	onPush: (task: Task) => void
	pendingID: string
}

/**
 * Renders one labelled list of task rows.
 * @returns The task list.
 */
function TaskList({
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
						{task.title}
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

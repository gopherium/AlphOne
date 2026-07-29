// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, InputControl, Text } from '@alphone/frontend-sdk'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { ValidationError, createTask, fetchDayTasks, patchTask } from './api'
import type { Task } from './api'
import { formatDay, isoDate } from './format'

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
 * Renders the day's task list with its quick add field and done group.
 * @returns The tasks screen.
 */
export function TasksScreen() {
	const date = isoDate(new Date())
	const queryClient = useQueryClient()
	const tasks = useInfiniteQuery({
		queryKey: ['tasks', 'day', date],
		queryFn: ({ pageParam }) => fetchDayTasks(date, pageParam),
		initialPageParam: '',
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
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

	return (
		<div className="alphone-tasks">
			<div className="alphone-tasks__header">
				<h1>Tasks</h1>
				<Text className="alphone-tasks__day">{formatDay(date)}</Text>
			</div>
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
					placeholder="Add a task for today"
					value={title}
					onChange={(event) => setTitle(event.target.value)}
				/>
				<Button type="submit" disabled={title.trim() === '' || add.isPending}>
					Add task
				</Button>
			</form>
			{add.isError ? <Text role="alert">{addErrorText(add.error)}</Text> : null}
			{change.isError ? <Text role="alert">The task could not be updated.</Text> : null}
			<TaskSections
				tasks={tasks}
				onChange={(task) => change.mutate(task)}
				pendingID={change.isPending ? change.variables.id : ''}
			/>
		</div>
	)
}

/**
 * Renders the loading, error, and loaded states of the day's tasks.
 * @returns The task sections.
 */
function TaskSections({
	tasks,
	onChange,
	pendingID,
}: {
	tasks: ReturnType<typeof useInfiniteQuery<Awaited<ReturnType<typeof fetchDayTasks>>>>
	onChange: (task: Task) => void
	pendingID: string
}) {
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
				<TaskList label="Open tasks" tasks={open} onChange={onChange} pendingID={pendingID} />
			)}
			{tasks.hasNextPage ? (
				<Button onClick={() => void tasks.fetchNextPage()} disabled={tasks.isFetchingNextPage}>
					Load more
				</Button>
			) : null}
			{done.length > 0 ? (
				<DoneGroup tasks={done} onChange={onChange} pendingID={pendingID} />
			) : null}
		</>
	)
}

/**
 * Renders the collapsible group of tasks completed on the day.
 * @returns The done group.
 */
function DoneGroup({
	tasks,
	onChange,
	pendingID,
}: {
	tasks: Task[]
	onChange: (task: Task) => void
	pendingID: string
}) {
	const [open, setOpen] = useState(false)
	return (
		<div className="alphone-tasks__done">
			<Button variant="tertiary" onClick={() => setOpen(!open)}>
				{`Done (${tasks.length})`}
			</Button>
			{open ? (
				<TaskList label="Done tasks" tasks={tasks} onChange={onChange} pendingID={pendingID} />
			) : null}
		</div>
	)
}

/**
 * Renders one labelled list of task rows.
 * @returns The task list.
 */
function TaskList({
	label,
	tasks,
	onChange,
	pendingID,
}: {
	label: string
	tasks: Task[]
	onChange: (task: Task) => void
	pendingID: string
}) {
	return (
		<ul className="alphone-tasks__list" aria-label={label}>
			{tasks.map((task) => (
				<li key={task.id} className="alphone-tasks__row" aria-label={task.title}>
					<Text className={task.status === 'done' ? 'alphone-tasks__title--done' : undefined}>
						{task.title}
					</Text>
					<Button
						variant="secondary"
						disabled={pendingID === task.id}
						onClick={() => onChange(task)}
					>
						{task.status === 'done' ? 'Reopen' : 'Complete'}
					</Button>
				</li>
			))}
		</ul>
	)
}

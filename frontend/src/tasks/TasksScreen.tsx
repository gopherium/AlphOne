// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	Collapsible,
	EmptyState,
	IconButton,
	InputControl,
	Notice,
	Stack,
	Text,
	chevronLeft,
	chevronRight,
	inbox,
} from '@alphone/frontend-sdk'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'

import {
	ValidationError,
	createTask,
	fetchDayTasks,
	fetchOverdueTasks,
	patchTask,
} from './api'
import type { Task } from './api'
import { formatDay, laterDate, shiftDate } from './format'
import { TaskList } from './TaskList'
import type { RowControls, TaskQuery } from './TaskList'

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
		queryKey: ['tasks', 'day', date, 'open'],
		queryFn: ({ pageParam }) => fetchDayTasks(date, pageParam, 'open'),
		initialPageParam: '',
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
	})
	const done = useInfiniteQuery({
		queryKey: ['tasks', 'day', date, 'done'],
		queryFn: ({ pageParam }) => fetchDayTasks(date, pageParam, 'done'),
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
		<Stack direction="column" gap="lg" className="alphone-tasks">
			<Stack direction="row" gap="md" align="center" justify="space-between">
				<Stack direction="column" gap="xs">
					<Text variant="heading-xl" render={<h1 />}>
						Tasks
					</Text>
					<Text variant="body-sm" className="alphone-tasks__day">
						{formatDay(date)}
					</Text>
				</Stack>
				<Stack direction="row" gap="sm" align="center">
					<DayNavigation date={date} today={today} />
					<Button
						variant="solid"
						render={<Link to="/tasks/new" search={{ date }} />}
						nativeButton={false}
					>
						New task
					</Button>
				</Stack>
			</Stack>
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
			{add.isError ? (
				<Notice.Root intent="error">
					<Notice.Description>{addErrorText(add.error)}</Notice.Description>
				</Notice.Root>
			) : null}
			{change.isError || push.isError ? (
				<Notice.Root intent="error">
					<Notice.Description>The task could not be updated.</Notice.Description>
				</Notice.Root>
			) : null}
			<TaskSections tasks={tasks} done={done} controls={controls} />
		</Stack>
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
function OverdueSection({ tasks, controls }: { tasks: TaskQuery; controls: RowControls }) {
	if (tasks.isPending) {
		return null
	}
	if (tasks.isError) {
		return (
			<Notice.Root intent="error">
				<Notice.Description>Overdue tasks could not be loaded.</Notice.Description>
			</Notice.Root>
		)
	}
	const rows = tasks.data.pages.flatMap((page) => page.tasks)
	if (rows.length === 0) {
		return null
	}
	return (
		<Stack direction="column" gap="sm" className="alphone-tasks__overdue">
			<Text variant="heading-sm" render={<h2 />}>
				Overdue
			</Text>
			<TaskList label="Overdue tasks" tasks={rows} controls={controls} showDueDate />
			{tasks.hasNextPage ? (
				<Button
					variant="minimal"
					tone="neutral"
					size="compact"
					onClick={() => void tasks.fetchNextPage()}
					disabled={tasks.isFetchingNextPage}
				>
					Load more overdue
				</Button>
			) : null}
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
	tasks: TaskQuery
	done: TaskQuery
	controls: RowControls
}) {
	if (tasks.isPending) {
		return <Text role="status">Loading tasks…</Text>
	}
	if (tasks.isError) {
		return (
			<Notice.Root intent="error">
				<Notice.Description>Tasks could not be loaded.</Notice.Description>
			</Notice.Root>
		)
	}
	const open = tasks.data.pages.flatMap((page) => page.tasks)
	return (
		<Stack direction="column" gap="lg">
			{open.length === 0 ? (
				<EmptyState.Root>
					<EmptyState.Icon icon={inbox} />
					<EmptyState.Title>Nothing due today.</EmptyState.Title>
					<EmptyState.Description>
						Add a task above, or open a contact and create one from their page.
					</EmptyState.Description>
				</EmptyState.Root>
			) : (
				<TaskList label="Open tasks" tasks={open} controls={controls} />
			)}
			{tasks.hasNextPage ? (
				<Button
					variant="minimal"
					tone="neutral"
					size="compact"
					onClick={() => void tasks.fetchNextPage()}
					disabled={tasks.isFetchingNextPage}
				>
					Load more
				</Button>
			) : null}
			<DoneGroup tasks={done} controls={controls} />
		</Stack>
	)
}

/**
 * Renders the collapsible group of tasks completed on the day.
 * @returns The done group.
 */
function DoneGroup({ tasks, controls }: { tasks: TaskQuery; controls: RowControls }) {
	if (tasks.isPending || tasks.isError) {
		return null
	}
	const rows = tasks.data.pages.flatMap((page) => page.tasks)
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
					{tasks.hasNextPage ? (
						<Button
							variant="minimal"
							tone="neutral"
							size="compact"
							onClick={() => void tasks.fetchNextPage()}
							disabled={tasks.isFetchingNextPage}
						>
							Load more done
						</Button>
					) : null}
				</Stack>
			</Collapsible.Panel>
		</Collapsible.Root>
	)
}

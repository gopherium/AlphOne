// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, InputControl, Notice, Text } from '@alphone/frontend-sdk'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { ValidationError, createTask, fetchContactTasks, patchTask } from './api'
import type { Task } from './api'
import { isoDate, laterDate, shiftDate } from './format'
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
 * Renders a contact's open tasks and the field that adds one.
 * @returns The contact tasks section.
 */
export function ContactTasks({ contactId }: { contactId: string }) {
	const today = isoDate(new Date())
	const queryClient = useQueryClient()
	const tasks = useInfiniteQuery({
		queryKey: ['tasks', 'contact', contactId],
		queryFn: ({ pageParam }) => fetchContactTasks(contactId, pageParam),
		initialPageParam: '',
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
	})
	const [title, setTitle] = useState('')
	const add = useMutation({
		mutationFn: () => createTask(title, today, { contact_id: contactId }),
		onSuccess: async () => {
			setTitle('')
			await queryClient.invalidateQueries({ queryKey: ['tasks'] })
		},
	})
	const change = useMutation({
		mutationFn: (task: Task) => patchTask(task.id, { status: 'done' }),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
	})
	const push = useMutation({
		mutationFn: (task: Task) =>
			patchTask(task.id, { due_on: shiftDate(laterDate(task.due_on, today), 1) }),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
	})

	return (
		<div className="alphone-tasks__contact-block">
			<h2>Tasks</h2>
			<form
				className="alphone-tasks__add"
				onSubmit={(event) => {
					event.preventDefault()
					add.mutate()
				}}
			>
				<InputControl
					label="New task for this contact"
					hideLabelFromVision
					placeholder="Add a task for this contact"
					value={title}
					onChange={(event) => setTitle(event.target.value)}
				/>
				<Button type="submit" disabled={title.trim() === '' || add.isPending}>
					Add task
				</Button>
			</form>
			{add.isError ? <Notice.Root intent="error">
					<Notice.Description>{addErrorText(add.error)}</Notice.Description>
				</Notice.Root> : null}
			{change.isError || push.isError ? (
				<Notice.Root intent="error">
					<Notice.Description>The task could not be updated.</Notice.Description>
				</Notice.Root>
			) : null}
			<ContactTaskList
				tasks={tasks}
				controls={{
					onChange: (task) => change.mutate(task),
					onPush: (task) => push.mutate(task),
					pendingID: change.isPending ? change.variables.id : '',
				}}
			/>
		</div>
	)
}

/**
 * Renders the loading, error, and loaded states of a contact's tasks.
 * @returns The contact task list.
 */
function ContactTaskList({
	tasks,
	controls,
}: {
	tasks: TaskQuery
	controls: RowControls
}) {
	if (tasks.isPending) {
		return <Text role="status">Loading tasks…</Text>
	}
	if (tasks.isError) {
		return <Notice.Root intent="error">
					<Notice.Description>Tasks could not be loaded.</Notice.Description>
				</Notice.Root>
	}
	const rows = tasks.data.pages.flatMap((page) => page.tasks)
	if (rows.length === 0) {
		return <Text role="status">No open tasks.</Text>
	}
	return (
		<>
			<TaskList label="Contact tasks" tasks={rows} controls={controls} showDueDate />
			{tasks.hasNextPage ? (
				<Button onClick={() => void tasks.fetchNextPage()} disabled={tasks.isFetchingNextPage}>
					Load more tasks
				</Button>
			) : null}
		</>
	)
}

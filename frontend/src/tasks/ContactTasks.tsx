// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	LoadingRows,
	Text,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { createTask, fetchContactTasks, patchTask } from './api'
import type { Task } from './api'
import { isoDate, laterDate, shiftDate } from './format'
import { TaskList } from './TaskList'
import type { RowControls, TaskQuery } from './TaskList'

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
			<Text variant="heading-sm" render={<h2 />}>
				Tasks
			</Text>
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
			{add.isError ? (
				<ErrorNotice>{validationMessage(add.error, 'The task could not be added.')}</ErrorNotice>
			) : null}
			{change.isError || push.isError ? (
				<ErrorNotice>The task could not be updated.</ErrorNotice>
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
		return <LoadingRows label="Loading tasks…" rows={3} />
	}
	if (tasks.isError) {
		return <ErrorNotice>Tasks could not be loaded.</ErrorNotice>
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

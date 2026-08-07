// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	Text,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useGraph } from '@alphone/frontend-sdk'
import type { ConnectionResult } from '@alphone/frontend-sdk'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { createTask, patchTask } from './api'
import { isoDate, laterDate, shiftDate } from './format'
import { TaskList } from './TaskList'
import type { RowControls, ListedTask } from './TaskList'

/** contactDetailOperation names the document a contact's tasks arrive in. */
const contactDetailOperation = 'ContactDetail'

/**
 * Renders a contact's open tasks and the field that adds one.
 * @returns The contact tasks section.
 */
export function ContactTasks({
	contactId,
	tasks,
}: {
	contactId: string
	tasks: ConnectionResult<ListedTask>
}) {
	const today = isoDate(new Date())
	const queryClient = useQueryClient()
	const graph = useGraph()
	const settled = async () => {
		graph.refetch([contactDetailOperation])
		await queryClient.invalidateQueries({ queryKey: ['tasks'] })
	}
	const [title, setTitle] = useState('')
	const add = useMutation({
		mutationFn: () => createTask(title, today, { contact_id: contactId }),
		onSuccess: async () => {
			setTitle('')
			await settled()
		},
	})
	const change = useMutation({
		mutationFn: (task: ListedTask) => patchTask(task.id, { status: 'done' }),
		onSuccess: settled,
	})
	const push = useMutation({
		mutationFn: (task: ListedTask) =>
			patchTask(task.id, { due_on: shiftDate(laterDate(task.due_on, today), 1) }),
		onSuccess: settled,
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
				<Button
					type="submit"
					disabled={title.trim() === '' || add.isPending}
					loading={add.isPending}
				>
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
 * Renders the empty and loaded states of a contact's tasks.
 * @returns The contact task list.
 */
function ContactTaskList({
	tasks,
	controls,
}: {
	tasks: ConnectionResult<ListedTask>
	controls: RowControls
}) {
	const rows = tasks.rows
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

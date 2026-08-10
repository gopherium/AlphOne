// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	Text,
	validationMessage,
} from '@alphone/frontend-sdk'
import { graphError, useGraph, useGraphMutation } from '@alphone/frontend-sdk'
import type { ConnectionResult } from '@alphone/frontend-sdk'
import { useState } from 'react'

import { isoDate, laterDate, shiftDate } from './format'
import { createTaskMutation, updateTaskMutation } from './operations'
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
	const graph = useGraph()
	const [title, setTitle] = useState('')
	const [pendingID, setPendingID] = useState('')
	const [add, runAdd] = useGraphMutation(createTaskMutation)
	const [change, runChange] = useGraphMutation(updateTaskMutation)
	const [push, runPush] = useGraphMutation(updateTaskMutation)
	const settled = () => graph.refetch([contactDetailOperation])
	const submitAdd = async () => {
		const result = await runAdd({ input: { title, dueOn: today, contactId } })
		if (result.data) {
			setTitle('')
			settled()
		}
	}
	const completeTask = async (task: ListedTask) => {
		setPendingID(task.id)
		const result = await runChange({ id: task.id, input: { status: 'done' } })
		setPendingID('')
		if (result.data) {
			settled()
		}
	}
	const pushTask = async (task: ListedTask) => {
		const result = await runPush({
			id: task.id,
			input: { dueOn: shiftDate(laterDate(task.dueOn, today), 1) },
		})
		if (result.data) {
			settled()
		}
	}

	return (
		<div className="alphone-tasks__contact-block">
			<Text variant="heading-sm" render={<h2 />}>
				Tasks
			</Text>
			<form
				className="alphone-tasks__add"
				onSubmit={(event) => {
					event.preventDefault()
					void submitAdd()
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
			<ContactTaskList
				tasks={tasks}
				controls={{
					onChange: (task) => void completeTask(task),
					onPush: (task) => void pushTask(task),
					pendingID,
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

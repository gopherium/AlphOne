// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	Text,
	__,
	validationMessage,
} from '@alphone/frontend-sdk'
import { graphError, useGraph, useGraphMutation } from '@alphone/frontend-sdk'
import type { ConnectionResult } from '@alphone/frontend-sdk'
import { useState } from 'react'

import { isValidDate, isoDate, laterDate, shiftDate } from './format'
import { createTaskMutation, updateTaskMutation } from './operations'
import { TaskList } from './TaskList'
import type { RowControls, ListedTask } from './TaskList'

/** contactDetailOperation names the document a contact's tasks arrive in. */
const contactDetailOperation = 'ContactDetail'

/**
 * Renders a contact's open tasks and the form that adds one.
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
	const [pendingID, setPendingID] = useState('')
	const [change, runChange] = useGraphMutation(updateTaskMutation)
	const [push, runPush] = useGraphMutation(updateTaskMutation)
	const settled = () => graph.refetch([contactDetailOperation])
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
				{__('Tasks', 'alphone')}
			</Text>
			<AddContactTaskForm contactId={contactId} onAdded={settled} />
			{change.error || push.error ? (
				<ErrorNotice>{__('The task could not be updated.', 'alphone')}</ErrorNotice>
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
 * Renders the form that adds a task to a contact on a chosen day.
 * @param props - The contact the task belongs to and what to call once it is added.
 * @returns The add task form and its error.
 */
function AddContactTaskForm({ contactId, onAdded }: { contactId: string; onAdded: () => void }) {
	const [title, setTitle] = useState('')
	const [picked, setPicked] = useState<string | null>(null)
	const [add, runAdd] = useGraphMutation(createTaskMutation)
	const dueOn = picked ?? isoDate(new Date())
	const submitAdd = async () => {
		const result = await runAdd({ input: { title, dueOn, contactId } })
		if (result.data) {
			setTitle('')
			setPicked(null)
			onAdded()
		}
	}

	return (
		<>
			<form
				className="alphone-tasks__add alphone-tasks__add--contact"
				onSubmit={(event) => {
					event.preventDefault()
					void submitAdd()
				}}
			>
				<InputControl
					label={__('New task for this contact', 'alphone')}
					hideLabelFromVision
					placeholder={__('Add a task for this contact', 'alphone')}
					value={title}
					onChange={(event) => setTitle(event.target.value)}
				/>
				<InputControl
					label={__('Due date', 'alphone')}
					type="date"
					value={dueOn}
					onChange={(event) => setPicked(event.target.value)}
				/>
				<Button
					type="submit"
					disabled={title.trim() === '' || !isValidDate(dueOn) || add.fetching}
					loading={add.fetching}
				>
					{__('Add task', 'alphone')}
				</Button>
			</form>
			{add.error ? (
				<ErrorNotice>
					{validationMessage(graphError(add.error), __('The task could not be added.', 'alphone'))}
				</ErrorNotice>
			) : null}
		</>
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
		return <Text role="status">{__('No open tasks.', 'alphone')}</Text>
	}
	return (
		<>
			<TaskList label={__('Contact tasks', 'alphone')} tasks={rows} controls={controls} showDueDate />
			{tasks.hasNextPage ? (
				<Button onClick={() => void tasks.fetchNextPage()} disabled={tasks.isFetchingNextPage}>
					{__('Load more tasks', 'alphone')}
				</Button>
			) : null}
		</>
	)
}

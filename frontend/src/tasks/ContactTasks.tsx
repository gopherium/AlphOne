// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	SectionTitle,
	Text,
	__,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import { graphError, useGraph, useGraphMutation } from '@alphone/frontend-sdk'
import type { ConnectionResult } from '@alphone/frontend-sdk'
import { useState } from 'react'

import { isValidDate, isoDate } from './format'
import { createTaskMutation } from './operations'
import { TaskList } from './TaskList'
import type { RowControls, ListedTask } from './TaskList'
import { useRowControls } from './useRowControls'

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
	const graph = useGraph()
	const settled = () => graph.refetch([contactDetailOperation])
	const { controls, failed } = useRowControls(isoDate(new Date()), settled)

	return (
		<div className="alphone-tasks__contact-block">
			<SectionTitle>{__('Tasks', 'alphone')}</SectionTitle>
			<AddContactTaskForm contactId={contactId} onAdded={settled} />
			{failed ? <ErrorNotice>{__('The task could not be updated.', 'alphone')}</ErrorNotice> : null}
			<ContactTaskList tasks={tasks} controls={controls} />
		</div>
	)
}

/**
 * Renders the form that adds a task to a contact on a chosen day.
 * @param props - The contact the task belongs to and what to call once it is added.
 * @returns The add task form and its error.
 */
function AddContactTaskForm({ contactId, onAdded }: { contactId: string; onAdded: () => void }) {
	const toaster = useToaster()
	const [title, setTitle] = useState('')
	const [picked, setPicked] = useState<string | null>(null)
	const [add, runAdd] = useGraphMutation(createTaskMutation)
	const dueOn = picked ?? isoDate(new Date())
	const submitAdd = async () => {
		const result = await runAdd({ input: { title, dueOn, contactId } })
		if (result.data) {
			setTitle('')
			setPicked(null)
			toaster.show(__('Task added.', 'alphone'))
			onAdded()
		}
	}

	return (
		<>
			<form
				className="godmin-form godmin-form--inline alphone-tasks__add--contact"
				onSubmit={(event) => {
					event.preventDefault()
					void submitAdd()
				}}
			>
				<div className="godmin-form__row">
					<InputControl
						className="godmin-form__grow"
						label={__('Task title', 'alphone')}
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
				</div>
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

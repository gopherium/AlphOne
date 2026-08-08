// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	LoadingScreen,
	PageScreen,
	Text,
	graphError,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'

import { taskDetailQuery, updateTaskMutation } from './operations'
import { PrioritySelect } from './PrioritySelect'

/** taskOperations names the documents a task write must refresh. */
const taskOperations = ['TaskDetail', 'DayTasks', 'OverdueTasks']

/** DetailedTask is one task as the detail document selects it. */
interface DetailedTask {
	id: string
	title: string
	status: string
	priority: number
	dueOn: string
	contact?: { id: string; name: string } | null
}

/**
 * Renders one task with its editable fields and contact link.
 * @returns The task screen.
 */
export function TaskScreen({ taskId }: { taskId: string }) {
	const [result] = useGraphQuery({
		query: taskDetailQuery,
		variables: { id: taskId },
		requestPolicy: 'cache-and-network',
	})
	const task = result.data?.task

	if (result.fetching && task === undefined) {
		return (
			<PageScreen title="Task">
				<LoadingScreen label="Loading task…" />
			</PageScreen>
		)
	}
	if (result.error || !task) {
		return <ErrorNotice>The task could not be loaded.</ErrorNotice>
	}
	return (
		<PageScreen title={task.title}>
			{task.contact ? <ContactLink contact={task.contact} /> : null}
			<TaskForm key={`${task.title}-${task.dueOn}`} task={task} />
		</PageScreen>
	)
}

/**
 * Renders the link to the contact a task belongs to.
 * @returns The contact link.
 */
function ContactLink({ contact }: { contact: { id: string; name: string } }) {
	return (
		<Text className="alphone-tasks__contact">
			<Link to="/contacts/$contactId" params={{ contactId: contact.id }}>
				{contact.name}
			</Link>
		</Text>
	)
}

/**
 * Renders the editable fields of a loaded task.
 * @returns The task form.
 */
function TaskForm({ task }: { task: DetailedTask }) {
	const graph = useGraph()
	const [title, setTitle] = useState(task.title)
	const [dueOn, setDueOn] = useState(task.dueOn)
	const [priority, setPriority] = useState(task.priority)
	const [save, runSave] = useGraphMutation(updateTaskMutation)
	const [toggle, runToggle] = useGraphMutation(updateTaskMutation)
	const runUpdate = async (
		run: typeof runSave,
		input: { title?: string; dueOn?: string; priority?: number; status?: string },
	) => {
		const result = await run({ id: task.id, input })
		if (result.data) {
			graph.refetch(taskOperations)
		}
	}

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				void runUpdate(runSave, { title, dueOn, priority })
			}}
		>
			<InputControl label="Title" value={title} onChange={(event) => setTitle(event.target.value)} />
			<InputControl
				label="Due date"
				type="date"
				value={dueOn}
				onChange={(event) => setDueOn(event.target.value)}
			/>
			<PrioritySelect value={priority} onChange={setPriority} />
			<div className="alphone-tasks__actions">
				<Button
					type="submit"
					disabled={title.trim() === '' || save.fetching}
					loading={save.fetching}
				>
					Save
				</Button>
				<Button
					variant="outline"
					loading={toggle.fetching}
					onClick={() =>
						void runUpdate(runToggle, { status: task.status === 'done' ? 'open' : 'done' })
					}
				>
					{task.status === 'done' ? 'Reopen' : 'Complete'}
				</Button>
			</div>
			{save.error ? (
				<ErrorNotice>
					{validationMessage(graphError(save.error), 'The task could not be saved.')}
				</ErrorNotice>
			) : null}
			{toggle.error ? <ErrorNotice>The task could not be saved.</ErrorNotice> : null}
		</form>
	)
}

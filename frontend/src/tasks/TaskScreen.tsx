// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	LoadingScreen,
	PageScreen,
	Text,
	useGraph,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'

import { patchTask } from './api'
import { taskDetailQuery } from './operations'
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
	const queryClient = useQueryClient()
	const graph = useGraph()
	const [title, setTitle] = useState(task.title)
	const [dueOn, setDueOn] = useState(task.dueOn)
	const [priority, setPriority] = useState(task.priority)
	const settled = async () => {
		graph.refetch(taskOperations)
		await queryClient.invalidateQueries({ queryKey: ['tasks'] })
	}
	const save = useMutation({
		mutationFn: () => patchTask(task.id, { title, due_on: dueOn, priority }),
		onSuccess: settled,
	})
	const toggle = useMutation({
		mutationFn: () =>
			patchTask(task.id, { status: task.status === 'done' ? 'open' : 'done' }),
		onSuccess: settled,
	})

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				save.mutate()
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
					disabled={title.trim() === '' || save.isPending}
					loading={save.isPending}
				>
					Save
				</Button>
				<Button variant="outline" loading={toggle.isPending} onClick={() => toggle.mutate()}>
					{task.status === 'done' ? 'Reopen' : 'Complete'}
				</Button>
			</div>
			{save.isError ? (
				<ErrorNotice>{validationMessage(save.error, 'The task could not be saved.')}</ErrorNotice>
			) : null}
			{toggle.isError ? <ErrorNotice>The task could not be saved.</ErrorNotice> : null}
		</form>
	)
}

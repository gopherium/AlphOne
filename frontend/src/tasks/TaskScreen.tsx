// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, InputControl, Notice, Text } from '@alphone/frontend-sdk'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'

import { fetchContact } from '../contacts/api'
import { ValidationError, fetchTask, patchTask } from './api'
import type { Task } from './api'
import { PrioritySelect } from './PrioritySelect'

/**
 * Copy for a failed save.
 * @param error - The mutation error.
 * @returns The message shown under the form.
 */
function saveErrorText(error: unknown): string {
	if (error instanceof ValidationError) {
		return error.message
	}
	return 'The task could not be saved.'
}

/**
 * Renders one task with its editable fields and contact link.
 * @returns The task screen.
 */
export function TaskScreen({ taskId }: { taskId: string }) {
	const task = useQuery({
		queryKey: ['tasks', 'detail', taskId],
		queryFn: () => fetchTask(taskId),
	})

	if (task.isPending) {
		return <Text role="status">Loading task…</Text>
	}
	if (task.isError) {
		return <Notice.Root intent="error">
					<Notice.Description>The task could not be loaded.</Notice.Description>
				</Notice.Root>
	}
	return (
		<div className="alphone-tasks">
			<h1>{task.data.title}</h1>
			{task.data.contact_id === null ? null : <ContactLink contactId={task.data.contact_id} />}
			<TaskForm key={`${task.data.title}-${task.data.due_on}`} task={task.data} />
		</div>
	)
}

/**
 * Renders the link to the contact a task belongs to.
 * @returns The contact link.
 */
function ContactLink({ contactId }: { contactId: string }) {
	const contact = useQuery({
		queryKey: ['contacts', 'detail', contactId],
		queryFn: () => fetchContact(contactId),
	})

	if (contact.isPending || contact.isError) {
		return null
	}
	return (
		<Text className="alphone-tasks__contact">
			<Link to="/contacts/$contactId" params={{ contactId }}>
				{contact.data.name}
			</Link>
		</Text>
	)
}

/**
 * Renders the editable fields of a loaded task.
 * @returns The task form.
 */
function TaskForm({ task }: { task: Task }) {
	const queryClient = useQueryClient()
	const [title, setTitle] = useState(task.title)
	const [dueOn, setDueOn] = useState(task.due_on)
	const [priority, setPriority] = useState(task.priority)
	const save = useMutation({
		mutationFn: () => patchTask(task.id, { title, due_on: dueOn, priority }),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
	})
	const toggle = useMutation({
		mutationFn: () =>
			patchTask(task.id, { status: task.status === 'done' ? 'open' : 'done' }),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
	})

	return (
		<form
			className="alphone-tasks__form"
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
				<Button type="submit" disabled={title.trim() === '' || save.isPending}>
					Save
				</Button>
				<Button variant="outline" disabled={toggle.isPending} onClick={() => toggle.mutate()}>
					{task.status === 'done' ? 'Reopen' : 'Complete'}
				</Button>
			</div>
			{save.isError ? <Notice.Root intent="error">
					<Notice.Description>{saveErrorText(save.error)}</Notice.Description>
				</Notice.Root> : null}
			{toggle.isError ? <Notice.Root intent="error">
					<Notice.Description>The task could not be saved.</Notice.Description>
				</Notice.Root> : null}
		</form>
	)
}

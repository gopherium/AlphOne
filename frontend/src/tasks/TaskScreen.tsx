// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	LoadingScreen,
	PageBreadcrumb,
	PageBreadcrumbs,
	PageScreen,
	Text,
	ValidatedInputControl,
	__,
	graphExtensions,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	useRefusedField,
	useToaster,
	writeFailure,
} from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'
import { useRef, useState } from 'react'

import { isValidDate, statusMessage, timeChange } from './format'
import { taskDetailQuery, updateTaskMutation } from './operations'
import { PrioritySelect } from './PrioritySelect'

/** titleRequired is the reason a task write fails for that is about its title. */
export const titleRequired = 'task_title_required'

/** taskOperations names the documents a task write must refresh. */
const taskOperations = ['TaskDetail', 'DayTasks', 'OverdueTasks']

/** DetailedTask is one task as the detail document selects it. */
interface DetailedTask {
	id: string
	title: string
	status: string
	priority: number
	dueOn: string
	dueTime: string | null
	contact?: { id: string; name: string } | null
}

/**
 * Renders the link back to the task list, on one day when the page knows it.
 * @returns The breadcrumbs of a task page.
 */
export function TaskBreadcrumbs({ date }: { date?: string }) {
	return (
		<PageBreadcrumbs label={__('Breadcrumbs', 'alphone')}>
			<PageBreadcrumb render={<Link to="/tasks" search={{ date }} activeOptions={{ exact: true }} />}>
				{__('Tasks', 'alphone')}
			</PageBreadcrumb>
		</PageBreadcrumbs>
	)
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
			<PageScreen title={__('Task', 'alphone')} breadcrumbs={<TaskBreadcrumbs />} headingLevel={2}>
				<LoadingScreen label={__('Loading task…', 'alphone')} />
			</PageScreen>
		)
	}
	if (result.error || !task) {
		return (
			<PageScreen title={__('Task', 'alphone')} breadcrumbs={<TaskBreadcrumbs />} headingLevel={2}>
				<ErrorNotice>{__('The task could not be loaded.', 'alphone')}</ErrorNotice>
			</PageScreen>
		)
	}
	return (
		<PageScreen title={task.title} breadcrumbs={<TaskBreadcrumbs date={task.dueOn} />} headingLevel={2}>
			{task.contact ? <ContactLink contact={task.contact} /> : null}
			<TaskForm key={`${task.title}-${task.dueOn}-${task.dueTime}`} task={task} />
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
	const toaster = useToaster()
	const form = useRef<HTMLFormElement>(null)
	const [title, setTitle] = useState(task.title)
	const [dueOn, setDueOn] = useState(task.dueOn)
	const [time, setTime] = useState(task.dueTime ?? '')
	const [priority, setPriority] = useState(task.priority)
	const [save, runSave] = useGraphMutation(updateTaskMutation)
	const [toggle, runToggle] = useGraphMutation(updateTaskMutation)
	const refusedTitle = useRefusedField(form, title)
	const status = task.status === 'done' ? 'open' : 'done'
	const settled = (message: string) => {
		toaster.show(message)
		graph.refetch(taskOperations)
	}
	const failed = (error: unknown) => {
		toaster.show(writeFailure(__('Could not save the task.', 'alphone'), error))
	}
	const submitSave = async () => {
		const result = await runSave({
			id: task.id,
			input: { title, dueOn, ...timeChange(time, task.dueTime), priority },
		})
		if (result.data) {
			settled(__('Task saved.', 'alphone'))
		} else if (graphExtensions(result.error).reason !== titleRequired || !refusedTitle.refuse(result.error, title)) {
			failed(result.error)
		}
	}
	const submitToggle = async () => {
		const result = await runToggle({ id: task.id, input: { status } })
		if (result.data) {
			settled(statusMessage(status))
		} else {
			failed(result.error)
		}
	}

	return (
		<form
			ref={form}
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				void submitSave()
			}}
		>
			<ValidatedInputControl
				label={__('Title', 'alphone')}
				value={title}
				customValidity={refusedTitle.validity}
				onChange={(event) => setTitle(event.target.value)}
			/>
			<InputControl
				label={__('Due date', 'alphone')}
				type="date"
				value={dueOn}
				onChange={(event) => setDueOn(event.target.value)}
			/>
			<InputControl
				label={__('Time', 'alphone')}
				type="time"
				value={time}
				onChange={(event) => setTime(event.target.value)}
			/>
			<PrioritySelect value={priority} onChange={setPriority} />
			<div className="alphone-tasks__actions">
				<Button
					type="submit"
					disabled={title.trim() === '' || !isValidDate(dueOn) || save.fetching}
					loading={save.fetching}
				>
					{__('Save', 'alphone')}
				</Button>
				<Button
					variant="outline"
					loading={toggle.fetching}
					onClick={() => void submitToggle()}
				>
					{task.status === 'done' ? __('Reopen', 'alphone') : __('Complete', 'alphone')}
				</Button>
			</div>
		</form>
	)
}

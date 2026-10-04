// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	__,
	_x,
	graphError,
	useGraph,
	useGraphMutation,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useRef, useState } from 'react'

import { addedMessage, isValidDate } from './format'
import { createTaskMutation, taskListReads } from './operations'
import { PrioritySelect } from './PrioritySelect'

/**
 * Renders the one row form that adds a task, starting on the given day at normal priority.
 * @returns The quick add form and its error.
 */
export function QuickAddForm({
	day,
	contactId,
	className,
	viewDay,
}: {
	day: string
	contactId?: string
	className?: string
	viewDay?: (date: string) => void
}) {
	const graph = useGraph()
	const toaster = useToaster()
	const titleField = useRef<HTMLInputElement>(null)
	const [title, setTitle] = useState('')
	const [picked, setPicked] = useState<{ on: string; value: string } | null>(null)
	const [priority, setPriority] = useState(0)
	const [add, runAdd] = useGraphMutation(createTaskMutation)
	const dueOn = picked !== null && picked.on === day ? picked.value : day
	const submitAdd = async () => {
		const result = await runAdd({ input: { title, dueOn, priority, contactId } })
		if (result.data) {
			setTitle('')
			setPicked(null)
			setPriority(0)
			if (viewDay === undefined || dueOn === day) {
				toaster.show(__('Task added.', 'alphone'))
			} else {
				toaster.show(addedMessage(dueOn), { label: _x('View', 'toast action', 'alphone'), onAct: () => viewDay(dueOn) })
			}
			graph.refetch(taskListReads)
			titleField.current?.focus()
		}
	}

	return (
		<>
			<form
				className={`godmin-form godmin-form--inline alphone-tasks__add${className === undefined ? '' : ` ${className}`}`}
				onSubmit={(event) => {
					event.preventDefault()
					void submitAdd()
				}}
			>
				<div className="godmin-form__row">
					<InputControl
						ref={titleField}
						className="godmin-form__grow"
						label={__('Task title', 'alphone')}
						value={title}
						onChange={(event) => setTitle(event.target.value)}
					/>
					<InputControl
						label={__('Due date', 'alphone')}
						type="date"
						value={dueOn}
						onChange={(event) => setPicked({ on: day, value: event.target.value })}
					/>
					<PrioritySelect value={priority} onChange={setPriority} />
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

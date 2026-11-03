// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	InputControl,
	ValidatedInputControl,
	__,
	_x,
	graphExtensions,
	useGraph,
	useGraphMutation,
	useRefusedField,
	useToaster,
	writeFailure,
} from '@alphone/frontend-sdk'
import { useLayoutEffect, useRef, useState } from 'react'

import { addedMessage, isValidDate, timeInput } from './format'
import { createTaskMutation, taskListReads } from './operations'
import { PrioritySelect } from './PrioritySelect'
import { titleRequired } from './TaskScreen'

/**
 * Renders the one row form that adds a task, starting on the given day at normal priority.
 * @returns The quick add form.
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
	const form = useRef<HTMLFormElement>(null)
	const titleField = useRef<HTMLInputElement>(null)
	const [title, setTitle] = useState('')
	const [picked, setPicked] = useState<{ on: string; value: string } | null>(null)
	const [time, setTime] = useState('')
	const [priority, setPriority] = useState(0)
	const [add, runAdd] = useGraphMutation(createTaskMutation)
	const refusedTitle = useRefusedField(form, title)
	const shownDay = useRef(day)
	useLayoutEffect(() => {
		shownDay.current = day
	})
	const dueOn = picked !== null && picked.on === day ? picked.value : day
	const submitAdd = async () => {
		const result = await runAdd({ input: { title, dueOn, ...timeInput(time), priority, contactId } })
		if (result.data) {
			setTitle('')
			setPicked(null)
			setTime('')
			setPriority(0)
			if (viewDay === undefined || dueOn === shownDay.current) {
				toaster.show(__('Task added.', 'alphone'))
			} else {
				toaster.show(addedMessage(dueOn), { label: _x('View', 'toast action', 'alphone'), onAct: () => viewDay(dueOn) })
			}
			graph.refetch(taskListReads)
			titleField.current?.focus()
		} else if (graphExtensions(result.error).reason !== titleRequired || !refusedTitle.refuse(result.error, title)) {
			toaster.show(writeFailure(__('Could not add the task.', 'alphone'), result.error))
		}
	}

	return (
		<form
			ref={form}
			className={`godmin-form godmin-form--inline alphone-tasks__add${className === undefined ? '' : ` ${className}`}`}
			onSubmit={(event) => {
				event.preventDefault()
				void submitAdd()
			}}
		>
			<div className="godmin-form__row">
				<div className="godmin-form__grow">
					<ValidatedInputControl
						ref={titleField}
						label={__('Task title', 'alphone')}
						value={title}
						customValidity={refusedTitle.validity}
						onChange={(event) => setTitle(event.target.value)}
					/>
				</div>
				<InputControl
					label={__('Due date', 'alphone')}
					type="date"
					value={dueOn}
					onChange={(event) => setPicked({ on: day, value: event.target.value })}
				/>
				<InputControl
					label={__('Time', 'alphone')}
					type="time"
					value={time}
					onChange={(event) => setTime(event.target.value)}
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
	)
}

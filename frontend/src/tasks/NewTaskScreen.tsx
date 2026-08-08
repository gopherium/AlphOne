// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	PageScreen,
	Text,
	graphError,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useState } from 'react'

import { contactsQuery } from '../contacts/operations'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { createTaskMutation } from './operations'
import { PrioritySelect } from './PrioritySelect'

const searchDebounceMs = 300
const contactSearchLimit = 20

interface PickableContact {
	id: string
	name: string
}

/**
 * Renders the full new-task form.
 * @returns The creation screen.
 */
export function NewTaskScreen({
	date,
	onCreated,
}: {
	date: string
	onCreated: (created: { id: string }) => void
}) {
	const graph = useGraph()
	const [title, setTitle] = useState('')
	const [dueOn, setDueOn] = useState(date)
	const [priority, setPriority] = useState(0)
	const [contact, setContact] = useState<PickableContact | null>(null)
	const [create, runCreate] = useGraphMutation(createTaskMutation)
	const submit = async () => {
		const result = await runCreate({
			input: {
				title,
				dueOn,
				priority,
				...(contact === null ? {} : { contactId: contact.id }),
			},
		})
		if (result.data) {
			graph.refetch(['DayTasks', 'OverdueTasks'])
			onCreated(result.data.createTask.task)
		}
	}

	return (
		<PageScreen title="New task">
			<form
				className="godmin-form"
				onSubmit={(event) => {
					event.preventDefault()
					void submit()
				}}
			>
				<InputControl
					label="Title"
					value={title}
					onChange={(event) => setTitle(event.target.value)}
				/>
				<InputControl
					label="Due date"
					type="date"
					value={dueOn}
					onChange={(event) => setDueOn(event.target.value)}
				/>
				<PrioritySelect value={priority} onChange={setPriority} />
				<ContactPicker contact={contact} onPick={setContact} />
				<Button
					type="submit"
					disabled={title.trim() === '' || create.fetching}
					loading={create.fetching}
				>
					Create task
				</Button>
				{create.error ? (
					<ErrorNotice>{validationMessage(graphError(create.error), 'The task could not be created.')}</ErrorNotice>
				) : null}
			</form>
		</PageScreen>
	)
}

/**
 * Renders the contact search and the chosen contact.
 * @returns The contact picker.
 */
function ContactPicker({
	contact,
	onPick,
}: {
	contact: PickableContact | null
	onPick: (contact: PickableContact | null) => void
}) {
	const [search, setSearch] = useState('')
	const query = useDebouncedValue(search, searchDebounceMs)
	const [found] = useGraphQuery({
		query: contactsQuery,
		variables: { q: query, first: contactSearchLimit },
		pause: query === '',
	})

	if (contact !== null) {
		return (
			<div className="alphone-tasks__picked">
				<Text>{contact.name}</Text>
				<Button variant="minimal" onClick={() => onPick(null)}>
					{`Remove ${contact.name}`}
				</Button>
			</div>
		)
	}
	return (
		<div className="alphone-tasks__picker">
			<InputControl
				label="Link a contact"
				value={search}
				onChange={(event) => setSearch(event.target.value)}
			/>
			<ContactResults
				contacts={found.data?.contacts.edges.map((edge) => edge.node) ?? []}
				settled={found.data !== undefined}
				onPick={onPick}
			/>
		</div>
	)
}

/**
 * Renders the contacts matching the picker's search.
 * @returns The contact results.
 */
function ContactResults({
	contacts,
	settled,
	onPick,
}: {
	contacts: readonly PickableContact[]
	settled: boolean
	onPick: (contact: PickableContact) => void
}) {
	if (!settled) {
		return null
	}
	if (contacts.length === 0) {
		return <Text role="status">No contacts found.</Text>
	}
	return (
		<ul className="alphone-tasks__results">
			{contacts.map((contact) => (
				<li key={contact.id}>
					<Button variant="minimal" onClick={() => onPick(contact)}>
						{contact.name}
					</Button>
				</li>
			))}
		</ul>
	)
}

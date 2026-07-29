// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, InputControl, Notice, Text } from '@alphone/frontend-sdk'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { fetchContacts } from '../contacts/api'
import type { Contact } from '../contacts/api'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { ValidationError, createTask } from './api'
import type { Task } from './api'
import { PrioritySelect } from './PrioritySelect'

const searchDebounceMs = 300

/**
 * Copy for a failed creation.
 * @param error - The mutation error.
 * @returns The message shown under the form.
 */
function createErrorText(error: unknown): string {
	if (error instanceof ValidationError) {
		return error.message
	}
	return 'The task could not be created.'
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
	onCreated: (created: Task) => void
}) {
	const queryClient = useQueryClient()
	const [title, setTitle] = useState('')
	const [dueOn, setDueOn] = useState(date)
	const [priority, setPriority] = useState(0)
	const [contact, setContact] = useState<Contact | null>(null)
	const create = useMutation({
		mutationFn: () =>
			createTask(title, dueOn, {
				priority,
				...(contact === null ? {} : { contact_id: contact.id }),
			}),
		onSuccess: (created) => {
			void queryClient.invalidateQueries({ queryKey: ['tasks'] })
			onCreated(created)
		},
	})

	return (
		<div className="alphone-tasks">
			<h1>New task</h1>
			<form
				className="alphone-tasks__form"
				onSubmit={(event) => {
					event.preventDefault()
					create.mutate()
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
				<Button type="submit" disabled={title.trim() === '' || create.isPending}>
					Create task
				</Button>
				{create.isError ? <Notice.Root intent="error">
					<Notice.Description>{createErrorText(create.error)}</Notice.Description>
				</Notice.Root> : null}
			</form>
		</div>
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
	contact: Contact | null
	onPick: (contact: Contact | null) => void
}) {
	const [search, setSearch] = useState('')
	const query = useDebouncedValue(search, searchDebounceMs)
	const contacts = useQuery({
		queryKey: ['contacts', 'list', query],
		queryFn: () => fetchContacts(query, ''),
		enabled: query !== '',
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
			<ContactResults contacts={contacts.data?.contacts ?? []} settled={contacts.isSuccess} onPick={onPick} />
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
	contacts: Contact[]
	settled: boolean
	onPick: (contact: Contact) => void
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

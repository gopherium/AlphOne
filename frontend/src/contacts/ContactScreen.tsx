// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	LoadingScreen,
	PageScreen,
	SelectControl,
	Text,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useConnection, useGraph } from '@alphone/frontend-sdk'
import type { ConnectionResult } from '@alphone/frontend-sdk'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { ContactTasks } from '../tasks/ContactTasks'
import type { ListedTask } from '../tasks/TaskList'
import { addIdentity, removeIdentity, renameContact } from './api'
import { channelItemOf, channelItems } from './channel'
import { formatCreated } from './format'
import { contactDetailQuery } from './operations'

const contactTasksPageSize = 50
const contactDetailOperation = 'ContactDetail'

/** ContactDetail is the contact the screen renders, identities included. */
export interface ContactDetail {
	id: string
	name: string
	createdAt: string
	identities: {
		id: string
		channel: string
		identifier: string
		displayName: string
	}[]
}

/**
 * Renders one contact's detail: rename form, identities, and creation date.
 * @returns The contact screen.
 */
export function ContactScreen({ contactId }: { contactId: string }) {
	const detail = useConnection({
		query: contactDetailQuery,
		variables: { id: contactId, first: contactTasksPageSize },
		select: (data) => data.contact?.tasks,
	})
	const contact = detail.data?.contact

	if (detail.isPending) {
		return (
			<PageScreen title="Contact">
				<LoadingScreen label="Loading contact…" />
			</PageScreen>
		)
	}
	if (detail.isError || !contact) {
		return <ErrorNotice>The contact could not be loaded.</ErrorNotice>
	}
	return (
		<PageScreen title={contact.name}>
			<RenameForm key={contact.name} contact={contact} />
			<Text variant="heading-sm" render={<h2 />}>
				Identities
			</Text>
			<IdentityList contact={contact} />
			<AddIdentityForm contact={contact} />
			<ContactTasks contactId={contact.id} tasks={toTaskRows(detail)} />
			<Text className="alphone-contacts__created">
				{`Created ${formatCreated(new Date(contact.createdAt))}`}
			</Text>
		</PageScreen>
	)
}

/**
 * Returns the callback refreshing the contact after a REST write.
 * @returns The refresh callback.
 */
function useContactRefresh() {
	const queryClient = useQueryClient()
	const graph = useGraph()
	return async () => {
		graph.refetch([contactDetailOperation])
		await queryClient.invalidateQueries({ queryKey: ['contacts'] })
	}
}

/** GraphTaskNode is one task as the contact detail document selects it. */
interface GraphTaskNode {
	id: string
	title: string
	status: string
	priority: number
	dueOn: string
}

/**
 * Maps the contact's task connection onto the rows the task list renders.
 * @param detail - The contact detail connection.
 * @returns The connection carrying list shaped rows.
 */
function toTaskRows(detail: ConnectionResult<GraphTaskNode>): ConnectionResult<ListedTask> {
	return {
		...detail,
		rows: detail.rows.map((node) => ({
			id: node.id,
			title: node.title,
			status: node.status,
			priority: node.priority,
			due_on: node.dueOn,
		})),
	}
}

/**
 * Renders the contact's identities, or a placeholder when none exist.
 * @returns The identity list.
 */
function IdentityList({ contact }: { contact: ContactDetail }) {
	const settled = useContactRefresh()
	const remove = useMutation({
		mutationFn: (identityId: string) => removeIdentity(contact.id, identityId),
		onSuccess: settled,
	})

	if (contact.identities.length === 0) {
		return <Text role="status">No identities yet.</Text>
	}
	return (
		<>
			<ul className="alphone-contacts__identities">
				{contact.identities.map((identity) => (
					<li key={identity.id}>
						<Text>
							{identity.displayName === ''
								? `${identity.channel}: ${identity.identifier}`
								: `${identity.channel}: ${identity.identifier} (${identity.displayName})`}
						</Text>
						<Button
							variant="minimal"
							size="small"
							aria-label={`Remove ${identity.identifier}`}
							disabled={remove.isPending}
							onClick={() => remove.mutate(identity.id)}
						>
							Remove
						</Button>
					</li>
				))}
			</ul>
			{remove.isError ? (
				<ErrorNotice>The identity could not be removed.</ErrorNotice>
			) : null}
		</>
	)
}

/**
 * Renders the form that attaches a new identity to the contact.
 * @returns The add identity form.
 */
function AddIdentityForm({ contact }: { contact: ContactDetail }) {
	const settled = useContactRefresh()
	const [channel, setChannel] = useState(channelItems[0])
	const [identifier, setIdentifier] = useState('')
	const [label, setLabel] = useState('')
	const add = useMutation({
		mutationFn: () =>
			addIdentity(contact.id, { channel: channel.value, identifier, displayName: label }),
		onSuccess: async () => {
			setIdentifier('')
			setLabel('')
			await settled()
		},
	})

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				add.mutate()
			}}
		>
			<SelectControl
				label="Channel"
				items={channelItems}
				value={channel}
				onValueChange={(item) => setChannel(channelItemOf(item))}
			/>
			<InputControl
				label="Value"
				value={identifier}
				onChange={(event) => setIdentifier(event.target.value)}
			/>
			<InputControl
				label="Label"
				value={label}
				onChange={(event) => setLabel(event.target.value)}
			/>
			<Button type="submit" disabled={identifier.trim() === '' || add.isPending}>
				Add identity
			</Button>
			{add.isError ? (
				<ErrorNotice>
					{validationMessage(add.error, 'The identity could not be added.')}
				</ErrorNotice>
			) : null}
		</form>
	)
}

/**
 * Renders the rename form for a loaded contact.
 * @returns The rename form.
 */
function RenameForm({ contact }: { contact: ContactDetail }) {
	const settled = useContactRefresh()
	const [name, setName] = useState(contact.name)
	const rename = useMutation({
		mutationFn: () => renameContact(contact.id, name),
		onSuccess: settled,
	})

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				rename.mutate()
			}}
		>
			<InputControl
				label="Name"
				value={name}
				onChange={(event) => setName(event.target.value)}
			/>
			<Button type="submit" disabled={name.trim() === '' || rename.isPending}>
				Save
			</Button>
			{rename.isError ? (
				<ErrorNotice>
					{validationMessage(rename.error, 'The contact could not be renamed.')}
				</ErrorNotice>
			) : null}
		</form>
	)
}

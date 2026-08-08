// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	LoadingScreen,
	PageScreen,
	SelectControl,
	Text,
	ValidationError,
	graphError,
	graphExtensions,
	useConnection,
	useGraph,
	useGraphMutation,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { ContactTasks } from '../tasks/ContactTasks'
import { toListedTasks } from '../tasks/TaskList'
import { channelItemOf, channelItems } from './channel'
import { formatCreated } from './format'
import {
	addContactIdentityMutation,
	contactDetailQuery,
	deleteContactIdentityMutation,
	renameContactMutation,
} from './operations'

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
			<ContactTasks contactId={contact.id} tasks={toListedTasks(detail)} />
			<Text className="alphone-contacts__created">
				{`Created ${formatCreated(new Date(contact.createdAt))}`}
			</Text>
		</PageScreen>
	)
}

/**
 * Returns the error an identity write should show, naming the owner of a claim.
 * @param error - The failure the mutation answered with.
 * @returns The mapped error, or undefined when the write succeeded.
 */
function identityError(error: Parameters<typeof graphError>[0]): Error | undefined {
	const owner = graphExtensions(error).ownerName
	if (typeof owner === 'string') {
		return new ValidationError(`Already on contact ${owner}.`)
	}
	return graphError(error)
}

/**
 * Returns the callback refreshing the contact after a write.
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

/**
 * Renders the contact's identities, or a placeholder when none exist.
 * @returns The identity list.
 */
function IdentityList({ contact }: { contact: ContactDetail }) {
	const settled = useContactRefresh()
	const [remove, runRemove] = useGraphMutation(deleteContactIdentityMutation)
	const removeIdentity = async (identityId: string) => {
		const result = await runRemove({ contactId: contact.id, identityId })
		if (result.data) {
			await settled()
		}
	}

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
							loading={remove.fetching}
							onClick={() => void removeIdentity(identity.id)}
						>
							Remove
						</Button>
					</li>
				))}
			</ul>
			{remove.error ? (
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
	const [add, runAdd] = useGraphMutation(addContactIdentityMutation)
	const submitIdentity = async () => {
		const result = await runAdd({
			contactId: contact.id,
			identity: { channel: channel.value, identifier, displayName: label },
		})
		if (result.data) {
			setIdentifier('')
			setLabel('')
			await settled()
		}
	}

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				void submitIdentity()
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
			<Button
				type="submit"
				disabled={identifier.trim() === '' || add.fetching}
				loading={add.fetching}
			>
				Add identity
			</Button>
			{add.error ? (
				<ErrorNotice>
					{validationMessage(identityError(add.error), 'The identity could not be added.')}
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
	const [rename, runRename] = useGraphMutation(renameContactMutation)
	const submitRename = async () => {
		const result = await runRename({ id: contact.id, name })
		if (result.data) {
			await settled()
		}
	}

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				void submitRename()
			}}
		>
			<InputControl
				label="Name"
				value={name}
				onChange={(event) => setName(event.target.value)}
			/>
			<Button
				type="submit"
				disabled={name.trim() === '' || rename.fetching}
				loading={rename.fetching}
			>
				Save
			</Button>
			{rename.error ? (
				<ErrorNotice>
					{validationMessage(graphError(rename.error), 'The contact could not be renamed.')}
				</ErrorNotice>
			) : null}
		</form>
	)
}

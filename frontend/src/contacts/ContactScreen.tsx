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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { ContactTasks } from '../tasks/ContactTasks'
import { addIdentity, fetchContact, removeIdentity, renameContact } from './api'
import type { ContactDetail } from './api'
import { channelItemOf, channelItems } from './channel'
import { formatCreated } from './format'

/**
 * Renders one contact's detail: rename form, identities, and creation date.
 * @returns The contact screen.
 */
export function ContactScreen({ contactId }: { contactId: string }) {
	const detail = useQuery({
		queryKey: ['contacts', 'detail', contactId],
		queryFn: () => fetchContact(contactId),
	})

	if (detail.isPending) {
		return (
			<PageScreen title="Contact">
				<LoadingScreen label="Loading contact…" />
			</PageScreen>
		)
	}
	if (detail.isError) {
		return <ErrorNotice>The contact could not be loaded.</ErrorNotice>
	}
	return (
		<PageScreen title={detail.data.name}>
			<RenameForm key={detail.data.name} contact={detail.data} />
			<Text variant="heading-sm" render={<h2 />}>
				Identities
			</Text>
			<IdentityList contact={detail.data} />
			<AddIdentityForm contact={detail.data} />
			<ContactTasks contactId={detail.data.id} />
			<Text className="alphone-contacts__created">
				{`Created ${formatCreated(detail.data.created_at)}`}
			</Text>
		</PageScreen>
	)
}

/**
 * Renders the contact's identities, or a placeholder when none exist.
 * @returns The identity list.
 */
function IdentityList({ contact }: { contact: ContactDetail }) {
	const queryClient = useQueryClient()
	const remove = useMutation({
		mutationFn: (identityId: string) => removeIdentity(contact.id, identityId),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['contacts'] }),
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
							{identity.display_name === ''
								? `${identity.channel}: ${identity.identifier}`
								: `${identity.channel}: ${identity.identifier} (${identity.display_name})`}
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
	const queryClient = useQueryClient()
	const [channel, setChannel] = useState(channelItems[0])
	const [identifier, setIdentifier] = useState('')
	const [label, setLabel] = useState('')
	const add = useMutation({
		mutationFn: () =>
			addIdentity(contact.id, { channel: channel.value, identifier, displayName: label }),
		onSuccess: () => {
			setIdentifier('')
			setLabel('')
			return queryClient.invalidateQueries({ queryKey: ['contacts'] })
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
	const queryClient = useQueryClient()
	const [name, setName] = useState(contact.name)
	const rename = useMutation({
		mutationFn: () => renameContact(contact.id, name),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['contacts'] }),
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

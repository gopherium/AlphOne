// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, InputControl, Text } from '@alphone/frontend-sdk'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { ValidationError, fetchContact, renameContact } from './api'
import type { ContactDetail } from './api'
import { formatCreated } from './format'

/**
 * Copy for a failed rename.
 * @param error - The mutation error.
 * @returns The message shown under the form.
 */
function renameErrorText(error: unknown): string {
	if (error instanceof ValidationError) {
		return error.message
	}
	return 'The contact could not be renamed.'
}

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
		return <Text role="status">Loading contact…</Text>
	}
	if (detail.isError) {
		return <Text role="alert">The contact could not be loaded.</Text>
	}
	return (
		<div className="alphone-contacts">
			<h1>{detail.data.name}</h1>
			<RenameForm key={detail.data.name} contact={detail.data} />
			<h2>Identities</h2>
			<IdentityList contact={detail.data} />
			<Text className="alphone-contacts__created">
				{`Created ${formatCreated(detail.data.created_at)}`}
			</Text>
		</div>
	)
}

/**
 * Renders the contact's identities, or a placeholder when none exist.
 * @returns The identity list.
 */
function IdentityList({ contact }: { contact: ContactDetail }) {
	if (contact.identities.length === 0) {
		return <Text role="status">No identities yet.</Text>
	}
	return (
		<ul className="alphone-contacts__identities">
			{contact.identities.map((identity) => (
				<li key={`${identity.channel}-${identity.identifier}`}>
					<Text>
						{identity.display_name === ''
							? `${identity.channel}: ${identity.identifier}`
							: `${identity.channel}: ${identity.identifier} (${identity.display_name})`}
					</Text>
				</li>
			))}
		</ul>
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
			className="alphone-contacts__form"
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
				<Text role="alert">{renameErrorText(rename.error)}</Text>
			) : null}
		</form>
	)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	EmptyState,
	ErrorNotice,
	InputControl,
	LoadingRows,
	LoadMore,
	PageScreen,
	__,
	people,
} from '@alphone/frontend-sdk'
import { useConnection } from '@alphone/frontend-sdk'
import type { ConnectionResult } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'

import { formatCreated } from './format'
import { contactsQuery } from './operations'
import { useDebouncedValue } from '../hooks/useDebouncedValue'

const searchDebounceMs = 300
const contactsPageSize = 50

interface ContactRow {
	id: string
	name: string
	createdAt: string
}

/**
 * Renders the searchable, cursor-paginated contact list.
 * @returns The contacts screen.
 */
export function ContactsScreen() {
	const [search, setSearch] = useState('')
	const query = useDebouncedValue(search, searchDebounceMs)
	const contacts = useConnection({
		query: contactsQuery,
		variables: { q: query === '' ? null : query, first: contactsPageSize },
		select: (data) => data.contacts,
	})

	return (
		<PageScreen
			title={__('Contacts', 'alphone')}
			actions={
				<Button variant="solid" render={<Link to="/contacts/new" />}>
					{__('New contact', 'alphone')}
				</Button>
			}
		>
			<InputControl
				label={__('Search contacts', 'alphone')}
				hideLabelFromVision
				className="alphone-contacts__search"
				value={search}
				onChange={(event) => setSearch(event.target.value)}
			/>
			<ContactRows contacts={contacts} />
		</PageScreen>
	)
}

/**
 * Renders the list body for the contacts query: loading, error, empty, or
 * the table with its load-more control.
 * @returns The list body.
 */
function ContactRows({ contacts }: { contacts: ConnectionResult<ContactRow> }) {
	if (contacts.isPending) {
		return <LoadingRows label={__('Loading contacts…', 'alphone')} />
	}
	if (contacts.isError) {
		return <ErrorNotice>{__('Contacts could not be loaded.', 'alphone')}</ErrorNotice>
	}
	const rows = contacts.rows
	if (rows.length === 0) {
		return (
			<EmptyState.Root className="godmin-empty">
				<EmptyState.Icon icon={people} />
				<EmptyState.Title>{__('No contacts found.', 'alphone')}</EmptyState.Title>
				<EmptyState.Description>
					{__('Try a different search, or add one with New contact.', 'alphone')}
				</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<>
			<div
				className="godmin-table-scroll godmin-arrival"
				role="region"
				aria-label={__('Contacts', 'alphone')}
				tabIndex={0}
			>
				<table className="godmin-table">
					<thead>
						<tr>
							<th>{__('Name', 'alphone')}</th>
							<th>{__('Created', 'alphone')}</th>
						</tr>
					</thead>
					<tbody>
						{rows.map((contact) => (
							<tr key={contact.id}>
								<td>
									<Link to="/contacts/$contactId" params={{ contactId: contact.id }}>
										{contact.name}
									</Link>
								</td>
								<td>{formatCreated(new Date(contact.createdAt))}</td>
							</tr>
						))}
					</tbody>
				</table>
			</div>
			<LoadMore query={contacts}>{__('Load more', 'alphone')}</LoadMore>
		</>
	)
}

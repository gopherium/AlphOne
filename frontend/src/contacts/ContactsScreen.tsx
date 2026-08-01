// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	EmptyState,
	ErrorNotice,
	InputControl,
	LoadMore,
	PageScreen,
	Text,
	people,
} from '@alphone/frontend-sdk'
import { useInfiniteQuery } from '@tanstack/react-query'
import type { InfiniteData, UseInfiniteQueryResult } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'

import { fetchContacts } from './api'
import type { ContactPage } from './api'
import { formatCreated } from './format'
import { useDebouncedValue } from '../hooks/useDebouncedValue'

const searchDebounceMs = 300

/**
 * Renders the searchable, cursor-paginated contact list.
 * @returns The contacts screen.
 */
export function ContactsScreen() {
	const [search, setSearch] = useState('')
	const query = useDebouncedValue(search, searchDebounceMs)
	const contacts = useInfiniteQuery({
		queryKey: ['contacts', 'list', query],
		queryFn: ({ pageParam }) => fetchContacts(query, pageParam),
		initialPageParam: '',
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
	})

	return (
		<PageScreen
			title="Contacts"
			actions={
				<Button variant="solid" render={<Link to="/contacts/new" />}>
					New contact
				</Button>
			}
		>
			<InputControl
				label="Search contacts"
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
function ContactRows({
	contacts,
}: {
	contacts: UseInfiniteQueryResult<InfiniteData<ContactPage>, Error>
}) {
	if (contacts.isPending) {
		return <Text role="status">Loading contacts…</Text>
	}
	if (contacts.isError) {
		return <ErrorNotice>Contacts could not be loaded.</ErrorNotice>
	}
	const rows = contacts.data.pages.flatMap((page) => page.contacts)
	if (rows.length === 0) {
		return (
			<EmptyState.Root>
				<EmptyState.Icon icon={people} />
				<EmptyState.Title>No contacts found.</EmptyState.Title>
				<EmptyState.Description>
					Try a different search, or add one with New contact.
				</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<>
			<table className="alphone-table">
				<thead>
					<tr>
						<th>Name</th>
						<th>Created</th>
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
							<td>{formatCreated(contact.created_at)}</td>
						</tr>
					))}
				</tbody>
			</table>
			<LoadMore query={contacts}>Load more</LoadMore>
		</>
	)
}

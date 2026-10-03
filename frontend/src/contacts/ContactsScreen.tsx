// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	EmptyState,
	ErrorNotice,
	PageScreen,
	__,
	openOnTap,
	people,
	useAdminSettings,
	useGraphQuery,
	useListView,
} from '@alphone/frontend-sdk'
import type { AdminSettings } from '@alphone/frontend-sdk'
import { DataViews } from '@alphone/frontend-sdk/dataviews'
import type { View } from '@alphone/frontend-sdk/dataviews'
import { Link, useNavigate } from '@tanstack/react-router'
import { useMemo, useState } from 'react'

import type { ContactPageQuery, ContactPageQueryVariables } from '../gql/graphql'
import { useContactActions } from './contactActions'
import { contactFields } from './contactFields'
import type { ContactRow } from './contactFields'
import { contactPageQuery } from './operations'

/** noContacts stands in for the list until the contacts arrive. */
const noContacts: ContactRow[] = []

/**
 * Renders what the list shows when no contact is on it.
 * @param props - Whether a search or a filter narrowed the list.
 * @returns The empty state.
 */
function ContactsEmpty({ narrowed }: { narrowed: boolean }) {
	return (
		<EmptyState.Root className="godmin-empty">
			<EmptyState.Icon icon={people} />
			{narrowed ? (
				<EmptyState.Title>{__('No contacts found.', 'alphone')}</EmptyState.Title>
			) : (
				<>
					<EmptyState.Title>{__('No contacts yet.', 'alphone')}</EmptyState.Title>
					<EmptyState.Description>{__('Add one with New contact.', 'alphone')}</EmptyState.Description>
				</>
			)}
		</EmptyState.Root>
	)
}

/**
 * Returns the channels the view narrows the contacts to.
 * @param filters - The filters the view holds.
 * @returns The channels picked, or null when none is.
 */
function channelsOf(filters: View['filters']): string[] | null {
	const picked = filters
		?.filter((filter) => filter.field === 'channels')
		.flatMap((filter) => (filter.value as string[] | undefined) ?? [])
	return picked?.length ? picked : null
}

/**
 * Returns the rows one page holds, the size the view names held under the contact page cap.
 * @param perPage - The page size the view holds.
 * @param settings - The admin settings, absent when they could not be read.
 * @returns The rows a page holds, or null for the size the graph serves by default.
 */
function pageSize(perPage: number | undefined, settings: AdminSettings | undefined): number | null {
	if (settings === undefined || perPage === undefined) {
		return null
	}
	return Math.min(perPage, settings.contactPageCap)
}

/**
 * Returns the limit and offset of the page a view shows, stepping by the size the graph served while none is known.
 * @param view - The view the list shows.
 * @param settings - The admin settings, absent when they could not be read.
 * @param served - The rows the graph last served on one page, zero before it served any.
 * @returns The limit and offset of the page.
 */
function pageWindow({ page, perPage }: View, settings: AdminSettings | undefined, served: number) {
	const limit = pageSize(perPage, settings)
	return { limit, offset: ((page as number) - 1) * (limit ?? served) }
}

/**
 * Builds the contact page request a view stands for.
 * @param view - The view the list shows.
 * @param settings - The admin settings, absent when they could not be read.
 * @param served - The rows the graph last served on one page, zero before it served any.
 * @returns The variables of the contact page query.
 */
function pageVariables(view: View, settings: AdminSettings | undefined, served: number): ContactPageQueryVariables {
	return {
		q: view.search || null,
		channels: channelsOf(view.filters),
		orderBy: view.sort?.field === 'name' ? 'NAME' : 'CREATED_AT',
		order: view.sort?.direction === 'asc' ? 'ASC' : 'DESC',
		...pageWindow(view, settings, served),
	}
}

/**
 * Returns the count and the pages DataViews pages through, by the rows the graph served on the page.
 * @param page - The page the graph served, absent until it arrives.
 * @returns The pagination info.
 */
function paginationOf(page: ContactPageQuery['contactPage'] | undefined) {
	if (page === undefined) {
		return { totalItems: 0, totalPages: 0 }
	}
	return { totalItems: page.total, totalPages: Math.ceil(page.total / page.limit) }
}

/**
 * Reads the contact page a view stands for, stepping by the size the graph served while the settings name none.
 * @param view - The view the list shows.
 * @param settings - The admin settings, absent until they arrive or when they could not be read.
 * @param sizing - Whether the list still waits for the admin settings.
 * @returns The query result beside the variables it was asked with.
 */
function useContactPage(view: View, settings: AdminSettings | undefined, sizing: boolean) {
	const [served, setServed] = useState(0)
	const variables = pageVariables(view, settings, served)
	const [result] = useGraphQuery({
		query: contactPageQuery,
		variables,
		pause: sizing,
		requestPolicy: 'cache-and-network',
	})
	const limit = result.data?.contactPage.limit
	if (limit !== undefined && limit !== served) {
		setServed(limit)
	}
	return { result, variables }
}

/**
 * Reports whether a search or a filter narrows the contacts a request asks for.
 * @param variables - The variables of the contact page query.
 * @returns Whether the request is narrowed.
 */
function narrowedBy(variables: ContactPageQueryVariables): boolean {
	return variables.q !== null || variables.channels !== null
}

/**
 * Renders the contacts as a list the server searches, filters, sorts and pages, with the actions each row offers.
 * @returns The contacts screen.
 */
export function ContactsScreen() {
	const { settings, failed } = useAdminSettings()
	const list = useListView({
		fields: ['channels', 'created'],
		titleField: 'name',
		descriptionField: 'identity',
		mediaField: 'avatar',
		sort: { field: 'created', direction: 'desc' },
		perPage: settings?.listPageSize,
	})
	const sizing = settings === undefined && !failed
	const { result, variables } = useContactPage(list.view, settings, sizing)
	const actions = useContactActions()
	const navigate = useNavigate()
	const onChangeSelection = openOnTap(list, (contactId) => {
		void navigate({ to: '/contacts/$contactId', params: { contactId } })
	})
	const fields = useMemo(() => contactFields(), [])
	const page = result.data?.contactPage

	return (
		<PageScreen
			title={__('Contacts', 'alphone')}
			subtitle={__('Manage the people you work with and how to reach them.', 'alphone')}
			actions={
				<Button variant="solid" size="compact" render={<Link to="/contacts/new" />}>
					{__('New contact', 'alphone')}
				</Button>
			}
			list
		>
			{result.error ? (
				<ErrorNotice>{__('Contacts could not be loaded.', 'alphone')}</ErrorNotice>
			) : (
				<DataViews<ContactRow>
					data={page?.items ?? noContacts}
					paginationInfo={paginationOf(page)}
					fields={fields}
					view={list.view}
					onChangeView={list.onChangeView}
					defaultLayouts={list.defaultLayouts}
					selection={list.selection}
					onChangeSelection={onChangeSelection}
					renderItemLink={({ item, ...props }) => (
						<Link to="/contacts/$contactId" params={{ contactId: item.id }} {...props} />
					)}
					actions={actions}
					isLoading={result.fetching || sizing}
					searchLabel={__('Search contacts…', 'alphone')}
					config={settings === undefined ? undefined : { perPageSizes: settings.listPageSizes }}
					empty={<ContactsEmpty narrowed={narrowedBy(variables)} />}
				/>
			)}
		</PageScreen>
	)
}

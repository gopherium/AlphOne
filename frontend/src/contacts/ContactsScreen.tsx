// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	EmptyState,
	ErrorNotice,
	PageScreen,
	__,
	openOnTap,
	paginationOf,
	people,
	useAdminSettings,
	useGraphQuery,
	useListView,
	useServerPaging,
} from '@alphone/frontend-sdk'
import type { AdminSettings, PagedView, PageWindow } from '@alphone/frontend-sdk'
import { DataViews } from '@alphone/frontend-sdk/dataviews'
import type { View } from '@alphone/frontend-sdk/dataviews'
import { Link, useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'

import type { ContactPageQueryVariables } from '../gql/graphql'
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
 * Returns the part of a view the server pages by, its page size only once the admin settings name the sizes.
 * @param view - The view the list shows.
 * @param settings - The admin settings, absent until they arrive or when they could not be read.
 * @returns The page alone while the settings are missing, so the graph serves its default size.
 */
function pagedView(view: View, settings: AdminSettings | undefined): PagedView {
	return settings === undefined ? { page: view.page } : view
}

/**
 * Builds the contact page request a view stands for.
 * @param view - The view the list shows.
 * @param rows - The limit and offset of the page.
 * @returns The variables of the contact page query.
 */
function pageVariables(view: View, rows: PageWindow): ContactPageQueryVariables {
	return {
		q: view.search || null,
		channels: channelsOf(view.filters),
		orderBy: view.sort?.field === 'name' ? 'NAME' : 'CREATED_AT',
		order: view.sort?.direction === 'asc' ? 'ASC' : 'DESC',
		...rows,
	}
}

/**
 * Reads the contact page a view stands for, stepping by the size the graph served.
 * @param view - The view the list shows.
 * @param settings - The admin settings, absent until they arrive or when they could not be read.
 * @param sizing - Whether the list still waits for the admin settings.
 * @returns The query result beside the variables it was asked with.
 */
function useContactPage(view: View, settings: AdminSettings | undefined, sizing: boolean) {
	const paging = useServerPaging(pagedView(view, settings), settings?.contactPageCap)
	const variables = pageVariables(view, paging.window)
	const [result] = useGraphQuery({
		query: contactPageQuery,
		variables,
		pause: sizing,
		requestPolicy: 'cache-and-network',
	})
	paging.record(result.data?.contactPage, result.operation?.variables.limit ?? null)
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

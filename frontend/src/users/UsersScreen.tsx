// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	EmptyState,
	ErrorNotice,
	MANAGE_USERS,
	PageScreen,
	__,
	can,
	people,
	useAdminSettings,
	useListView,
	useSession,
} from '@alphone/frontend-sdk'
import { DataViews, filterSortAndPaginate } from '@alphone/frontend-sdk/dataviews'
import { fetchUsers, usersQueryKey } from '@gopherium/react-auth/admin'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useMemo, useState } from 'react'

import type { Account } from '../auth/graphTransport'
import { UserTabs } from './UserTabs'
import { useUserActions } from './userActions'
import { userFields } from './userFields'

/** noAccounts stands in for the list until the accounts arrive. */
const noAccounts: Account[] = []

/**
 * Renders what the list shows when no account is on it.
 * @param props - Whether accounts exist that the view narrowed away.
 * @returns The empty state.
 */
function UsersEmpty({ narrowed }: { narrowed: boolean }) {
	return (
		<EmptyState.Root className="godmin-empty">
			<EmptyState.Icon icon={people} />
			{narrowed ? (
				<EmptyState.Title>{__('No users found.', 'alphone')}</EmptyState.Title>
			) : (
				<>
					<EmptyState.Title>{__('No users yet.', 'alphone')}</EmptyState.Title>
					<EmptyState.Description>{__('Add one with New user.', 'alphone')}</EmptyState.Description>
				</>
			)}
		</EmptyState.Root>
	)
}

/**
 * Renders the button beside the title, New user, only for a reader who manages users.
 * @param manages - Whether the reader manages users.
 * @returns The page action, or nothing.
 */
function usersAction(manages: boolean) {
	if (!manages) {
		return undefined
	}
	return (
		<Button variant="solid" size="compact" render={<Link to="/users/new" />}>
			{__('New user', 'alphone')}
		</Button>
	)
}

/**
 * Renders the accounts as a list to search, filter, sort and page, with the actions a manager may take.
 * @returns The users screen.
 */
export function UsersScreen() {
	const session = useSession()
	const manages = can(session, MANAGE_USERS)
	const users = useQuery({
		queryKey: usersQueryKey,
		queryFn: ({ signal }) => fetchUsers(signal) as Promise<Account[]>,
	})
	const { settings, failed } = useAdminSettings()
	const list = useListView({
		fields: ['status', 'role', 'created'],
		titleField: 'name',
		descriptionField: 'email',
		mediaField: 'avatar',
		sort: { field: 'created', direction: 'desc' },
		perPage: settings?.listPageSize,
	})
	const [failure, setFailure] = useState<string>()
	const actions = useUserActions(session, setFailure)
	const accounts = users.data ?? noAccounts
	const fields = useMemo(() => userFields(accounts), [accounts])
	const shown = filterSortAndPaginate(accounts, list.view, fields)
	const sizing = list.view.perPage === undefined && !failed

	return (
		<PageScreen
			title={__('Users', 'alphone')}
			subtitle={__('Manage who can sign in and what they can do.', 'alphone')}
			actions={usersAction(manages)}
			tabs={<UserTabs current="users" />}
			list
		>
			{failure === undefined ? null : <ErrorNotice>{failure}</ErrorNotice>}
			{users.isError ? (
				<ErrorNotice>{__('Users could not be loaded.', 'alphone')}</ErrorNotice>
			) : (
				<DataViews<Account>
					data={shown.data}
					paginationInfo={shown.paginationInfo}
					fields={fields}
					view={list.view}
					onChangeView={list.onChangeView}
					defaultLayouts={list.defaultLayouts}
					selection={list.selection}
					onChangeSelection={list.onChangeSelection}
					actions={actions}
					isLoading={users.isPending || sizing}
					searchLabel={__('Search users…', 'alphone')}
					config={settings === undefined ? undefined : { perPageSizes: settings.listPageSizes }}
					empty={<UsersEmpty narrowed={accounts.length > 0} />}
				/>
			)}
		</PageScreen>
	)
}

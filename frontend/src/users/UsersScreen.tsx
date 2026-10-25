// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	ListEmpty,
	PageScreen,
	__,
	people,
	useAdminSettings,
	useBulkRun,
	useListRun,
	useListView,
	useSession,
} from '@alphone/frontend-sdk'
import { DataViews, filterSortAndPaginate } from '@alphone/frontend-sdk/dataviews'
import { fetchUsers, usersQueryKey } from '@gopherium/react-auth/admin'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useCallback, useMemo } from 'react'

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
	if (narrowed) {
		return <ListEmpty icon={people} title={__('No users found.', 'alphone')} />
	}
	return (
		<ListEmpty
			icon={people}
			title={__('No users yet.', 'alphone')}
			hint={__('Add one with New user.', 'alphone')}
		/>
	)
}

/**
 * Renders the accounts as a list to search, filter, sort and page, with the actions a manager may take.
 * @returns The users screen.
 */
export function UsersScreen() {
	const session = useSession()
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
	const queryClient = useQueryClient()
	const { run } = useListRun('users', list.onChangeSelection)
	const reload = useCallback(() => queryClient.invalidateQueries({ queryKey: usersQueryKey }), [queryClient])
	const actions = useUserActions(session, useBulkRun<Account>(run, reload))
	const accounts = users.data ?? noAccounts
	const fields = useMemo(() => userFields(accounts), [accounts])
	const shown = filterSortAndPaginate(accounts, list.view, fields)
	const sizing = list.view.perPage === undefined && !failed

	return (
		<PageScreen
			title={__('Users', 'alphone')}
			headingLevel={2}
			subtitle={__('Manage who can sign in and what they can do.', 'alphone')}
			actions={
				<Button variant="solid" size="compact" render={<Link to="/users/new" />}>
					{__('New user', 'alphone')}
				</Button>
			}
			tabs={<UserTabs current="users" />}
			list
		>
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

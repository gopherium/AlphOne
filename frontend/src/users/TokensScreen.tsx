// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	EmptyState,
	ErrorNotice,
	PageScreen,
	__,
	key,
	useAdminSettings,
	useBulkRun,
	useGraph,
	useGraphQuery,
	useListRun,
	useListView,
} from '@alphone/frontend-sdk'
import { DataViews, filterSortAndPaginate } from '@alphone/frontend-sdk/dataviews'
import { usersNavItem } from '@gopherium/react-auth/wpds'
import { Link } from '@tanstack/react-router'
import { useCallback, useMemo } from 'react'
import type { ReactNode } from 'react'

import { useReachable } from '../menu/reach'
import { UserTabs } from './UserTabs'
import { useTokenActions } from './tokenActions'
import { tokenFields } from './tokenFields'
import type { ApiToken } from './tokenFields'
import { apiTokensQuery } from './tokenOperations'

/** noTokens stands in for the list until the tokens arrive. */
const noTokens: ApiToken[] = []

/**
 * Renders what the list shows when no token is on it.
 * @param props - Whether tokens exist that the view narrowed away.
 * @returns The empty state.
 */
function TokensEmpty({ narrowed }: { narrowed: boolean }) {
	return (
		<EmptyState.Root className="godmin-empty">
			<EmptyState.Icon icon={key} />
			{narrowed ? (
				<EmptyState.Title>{__('No tokens found.', 'alphone')}</EmptyState.Title>
			) : (
				<>
					<EmptyState.Title>{__('No API tokens yet.', 'alphone')}</EmptyState.Title>
					<EmptyState.Description>{__('Add one with New token.', 'alphone')}</EmptyState.Description>
				</>
			)}
		</EmptyState.Root>
	)
}

/**
 * Returns the title and tabs of the tokens page, the ones of the users page for a reader who opens Users.
 * @param opensUsers - Whether the reader may open the Users screen.
 * @returns The page title, and the tabs when the page has them.
 */
function tokensHeading(opensUsers: boolean): { title: string; tabs?: ReactNode } {
	if (opensUsers) {
		return { title: __('Users', 'alphone'), tabs: <UserTabs current="tokens" /> }
	}
	return { title: __('API tokens', 'alphone') }
}

/**
 * Renders the caller's own API tokens as a list to search, filter, sort and page, each one revocable.
 * @returns The tokens screen.
 */
export function TokensScreen() {
	const heading = tokensHeading(useReachable()(usersNavItem))
	const graph = useGraph()
	const [result] = useGraphQuery({
		query: apiTokensQuery,
		requestPolicy: 'cache-and-network',
	})
	const { settings, failed } = useAdminSettings()
	const list = useListView({
		fields: ['created', 'lastUsed', 'expires'],
		titleField: 'name',
		descriptionField: 'scopes',
		sort: { field: 'created', direction: 'desc' },
		perPage: settings?.listPageSize,
	})
	const { run } = useListRun('tokens', list.onChangeSelection)
	const reload = useCallback(
		() => graph.client.query(apiTokensQuery, {}, { requestPolicy: 'network-only' }).toPromise(),
		[graph],
	)
	const actions = useTokenActions(useBulkRun<ApiToken>(run, reload))
	const tokens = result.data?.apiTokens ?? noTokens
	const fields = useMemo(() => tokenFields(tokens), [tokens])
	const shown = filterSortAndPaginate(tokens, list.view, fields)
	const sizing = list.view.perPage === undefined && !failed

	return (
		<PageScreen
			title={heading.title}
			subtitle={__('Manage the tokens your programs sign in with.', 'alphone')}
			actions={
				<Button variant="solid" size="compact" render={<Link to="/users/tokens/new" />}>
					{__('New token', 'alphone')}
				</Button>
			}
			tabs={heading.tabs}
			list
		>
			{result.error ? (
				<ErrorNotice>{__('Tokens could not be loaded.', 'alphone')}</ErrorNotice>
			) : (
				<DataViews<ApiToken>
					data={shown.data}
					paginationInfo={shown.paginationInfo}
					fields={fields}
					view={list.view}
					onChangeView={list.onChangeView}
					defaultLayouts={list.defaultLayouts}
					selection={list.selection}
					onChangeSelection={list.onChangeSelection}
					actions={actions}
					isLoading={result.data === undefined || sizing}
					searchLabel={__('Search tokens…', 'alphone')}
					config={settings === undefined ? undefined : { perPageSizes: settings.listPageSizes }}
					empty={<TokensEmpty narrowed={tokens.length > 0} />}
				/>
			)}
		</PageScreen>
	)
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	EmptyState,
	ErrorNotice,
	PageScreen,
	__,
	key,
	useAdminSettings,
	useGraphQuery,
	useListView,
} from '@alphone/frontend-sdk'
import { DataViews, filterSortAndPaginate } from '@alphone/frontend-sdk/dataviews'
import { Link } from '@tanstack/react-router'
import { useCallback, useMemo, useState } from 'react'

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
 * Renders the caller's own API tokens as a list to search, filter, sort and page, each one revocable.
 * @returns The tokens screen.
 */
export function TokensScreen() {
	const [result, refetch] = useGraphQuery({
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
	const [failure, setFailure] = useState<string>()
	const reload = useCallback(() => refetch({ requestPolicy: 'network-only' }), [refetch])
	const actions = useTokenActions(setFailure, reload)
	const tokens = result.data?.apiTokens ?? noTokens
	const fields = useMemo(() => tokenFields(tokens), [tokens])
	const shown = filterSortAndPaginate(tokens, list.view, fields)
	const sizing = list.view.perPage === undefined && !failed

	return (
		<PageScreen
			title={__('Users', 'alphone')}
			subtitle={__('Manage the tokens your programs sign in with.', 'alphone')}
			actions={
				<Button variant="solid" size="compact" render={<Link to="/users/tokens/new" />}>
					{__('New token', 'alphone')}
				</Button>
			}
			tabs={<UserTabs current="tokens" />}
			list
		>
			{failure === undefined ? null : <ErrorNotice>{failure}</ErrorNotice>}
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

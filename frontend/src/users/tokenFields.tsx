// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, formatDate } from '@alphone/frontend-sdk'
import type { Field } from '@alphone/frontend-sdk/dataviews'

import type { ApiTokensQuery } from '../gql/graphql'
import { formatExpiry, formatLastUsed } from './tokenFormat'

/** ApiToken is one token as the tokens list shows it. */
export type ApiToken = ApiTokensQuery['apiTokens'][number]

/** endless stands in for the end of a token that never expires. */
const endless = Number.MAX_SAFE_INTEGER

/**
 * Returns the moment as milliseconds, or the fallback when the moment is absent.
 * @param at - The moment, or null.
 * @param fallback - The value an absent moment sorts as.
 * @returns The milliseconds.
 */
function momentOf(at: string | null, fallback: number): number {
	return at === null ? fallback : new Date(at).getTime()
}

/**
 * Builds the fields the tokens list shows, offering as scopes only the ones the tokens hold.
 * @param tokens - Every token the list holds.
 * @returns The fields.
 */
export function tokenFields(tokens: readonly ApiToken[]): Field<ApiToken>[] {
	const scopes = [...new Set(tokens.flatMap((token) => token.scopes))]
		.sort((a, b) => a.localeCompare(b))
		.map((scope) => ({ value: scope, label: scope }))
	return [
		{
			id: 'name',
			label: __('Name', 'alphone'),
			type: 'text',
			enableGlobalSearch: true,
			enableHiding: false,
			filterBy: false,
		},
		{
			id: 'scopes',
			label: __('Scopes', 'alphone'),
			type: 'array',
			elements: scopes,
			enableSorting: false,
			filterBy: { operators: ['isAny'] },
		},
		{
			id: 'created',
			label: __('Created', 'alphone'),
			getValue: ({ item }) => new Date(item.createdAt).getTime(),
			render: ({ item }) => formatDate(item.createdAt),
			filterBy: false,
		},
		{
			id: 'lastUsed',
			label: __('Last used', 'alphone'),
			getValue: ({ item }) => momentOf(item.lastUsedAt, 0),
			render: ({ item }) => formatLastUsed(item.lastUsedAt),
			filterBy: false,
		},
		{
			id: 'expires',
			label: __('Expires', 'alphone'),
			getValue: ({ item }) => momentOf(item.expiresAt, endless),
			render: ({ item }) => formatExpiry(item.expiresAt, new Date()),
			filterBy: false,
		},
	]
}

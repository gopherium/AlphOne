// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, __ } from '@alphone/frontend-sdk'
import { listDate } from '@alphone/frontend-sdk/dataviews'
import type { Field } from '@alphone/frontend-sdk/dataviews'

import type { ApiTokensQuery } from '../gql/graphql'

/** ApiToken is one token as the tokens list shows it. */
export type ApiToken = ApiTokensQuery['apiTokens'][number]

/** endless stands in for the end of a token that never expires. */
const endless = Number.MAX_SAFE_INTEGER

/**
 * Formats the moment a token was last presented.
 * @param at - The moment, or null when it never acted.
 * @returns The list date, or a plain statement that it never acted.
 */
function formatLastUsed(at: string | null | undefined): string {
	return at === null || at === undefined ? __('Never used', 'alphone') : listDate(at)
}

/**
 * Formats the moment a token ends.
 * @param at - The moment, or null when it never ends.
 * @param now - The moment to judge a lapsed token against.
 * @returns The list date, or a plain statement of an endless or lapsed token.
 */
function formatExpiry(at: string | null | undefined, now: Date): string {
	if (at === null || at === undefined) {
		return __('Never expires', 'alphone')
	}
	const ends = new Date(at)
	return ends.getTime() <= now.getTime() ? _x('Expired', 'token expiry', 'alphone') : listDate(ends)
}

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
			render: ({ item }) => listDate(item.createdAt),
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

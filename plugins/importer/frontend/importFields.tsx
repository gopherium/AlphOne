// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, __, _x, formatDate, formatNumber, formatTime } from '@alphone/frontend-sdk'
import type { Field } from '@alphone/frontend-sdk/dataviews'
import { Link } from '@tanstack/react-router'

import type { ImportsQuery } from './gql/graphql'

/** ImportSummary is one import as the imports list shows it. */
export type ImportSummary = ImportsQuery['imports'][number]

/** ImportState is a state an import stands in, as the server stores it. */
type ImportState = 'ready' | 'committing' | 'committed'

/** BadgeIntent is the badge intent a state is drawn with. */
type BadgeIntent = 'informational' | 'low' | 'stable' | 'none'

/** states lists every state an import stands in, in the order the filter offers them. */
const states: readonly ImportState[] = ['ready', 'committing', 'committed']

/** stateIntents names the badge intent each known state is drawn with. */
const stateIntents: Record<string, BadgeIntent> = {
	ready: 'informational',
	committing: 'low',
	committed: 'stable',
}

/**
 * Returns the label each state reads as, read fresh so the loaded catalogue answers.
 * @returns The labels, keyed by the state the server stores.
 */
function stateLabels(): Record<string, string> {
	return {
		ready: _x('Ready', 'import state', 'alphone-importer'),
		committing: _x('Importing', 'import state', 'alphone-importer'),
		committed: _x('Imported', 'import state', 'alphone-importer'),
	}
}

/**
 * Returns the moment an import started, written as a day and a time in the format locale.
 * @param at - The moment the import was stored.
 * @returns The moment, such as 03/08/2026 09:05.
 */
function startedAt(at: string): string {
	return `${formatDate(at)} ${formatTime(at)}`
}

/**
 * Builds the field of one count an import keeps, a whole number set on the right.
 * @param id - The field the count is read from.
 * @param label - The column heading.
 * @returns The field.
 */
function countField(
	id: 'rowCount' | 'importedCount' | 'skippedCount' | 'failedCount',
	label: string,
): Field<ImportSummary> {
	return {
		id,
		label,
		type: 'integer',
		render: ({ item }) => formatNumber(item[id]),
		filterBy: false,
	}
}

/**
 * Builds the fields the imports list shows.
 * @returns The fields.
 */
export function importFields(): Field<ImportSummary>[] {
	const labels = stateLabels()
	return [
		{
			id: 'filename',
			label: __('File', 'alphone-importer'),
			type: 'text',
			enableGlobalSearch: true,
			enableHiding: false,
			filterBy: false,
			render: ({ item }) => (
				<Link to="/import/$importId" params={{ importId: item.id }}>
					{item.filename}
				</Link>
			),
		},
		{
			id: 'state',
			label: __('State', 'alphone-importer'),
			type: 'text',
			elements: states.map((state) => ({ value: state, label: labels[state] })),
			filterBy: { operators: ['isAny'] },
			render: ({ item }) => (
				<Badge intent={stateIntents[item.state] ?? 'none'}>{labels[item.state] ?? item.state}</Badge>
			),
		},
		countField('rowCount', __('Rows', 'alphone-importer')),
		countField('importedCount', _x('Imported', 'import count', 'alphone-importer')),
		countField('skippedCount', _x('Skipped', 'import count', 'alphone-importer')),
		countField('failedCount', _x('Failed', 'import count', 'alphone-importer')),
		{
			id: 'started',
			label: __('Started', 'alphone-importer'),
			getValue: ({ item }) => new Date(item.createdAt).getTime(),
			render: ({ item }) => startedAt(item.createdAt),
			filterBy: false,
		},
	]
}

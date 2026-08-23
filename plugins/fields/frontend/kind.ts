// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x } from '@alphone/frontend-sdk'

import type { FieldKind } from './gql/graphql'

/**
 * Returns the kind options, read fresh so the loaded catalogue answers.
 * @returns The options, in menu order.
 */
export function kindItems(): { value: FieldKind; label: string }[] {
	return [
		{ value: 'TEXT', label: _x('Text', 'field kind', 'alphone-fields') },
		{ value: 'LONGTEXT', label: _x('Long text', 'field kind', 'alphone-fields') },
		{ value: 'NUMBER', label: _x('Number', 'field kind', 'alphone-fields') },
		{ value: 'BOOLEAN', label: _x('Yes or no', 'field kind', 'alphone-fields') },
		{ value: 'DATE', label: _x('Date', 'field kind', 'alphone-fields') },
		{ value: 'SELECT', label: _x('Choice', 'field kind', 'alphone-fields') },
	]
}

/**
 * Reads the kind item a select item stands for.
 * @param item - The chosen item, or null when the selection is cleared.
 * @returns The matching kind item, or the text item as the default.
 */
export function kindOf(item: { value: string | null } | null) {
	const items = kindItems()
	return items.find((candidate) => candidate.value === item?.value) ?? items[0]
}

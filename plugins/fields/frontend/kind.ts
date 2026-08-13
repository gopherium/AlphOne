// SPDX-License-Identifier: AGPL-3.0-or-later

import type { FieldKind } from './gql/graphql'

export const kindItems: { value: FieldKind; label: string }[] = [
	{ value: 'TEXT', label: 'Text' },
	{ value: 'LONGTEXT', label: 'Long text' },
	{ value: 'NUMBER', label: 'Number' },
	{ value: 'BOOLEAN', label: 'Yes or no' },
	{ value: 'DATE', label: 'Date' },
	{ value: 'SELECT', label: 'Choice' },
]

/**
 * Reads the kind item a select item stands for.
 * @param item - The chosen item, or null when the selection is cleared.
 * @returns The matching kind item, or the text item as the default.
 */
export function kindOf(item: { value: string | null } | null) {
	return kindItems.find((candidate) => candidate.value === item?.value) ?? kindItems[0]
}

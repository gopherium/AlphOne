// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x } from '@alphone/frontend-sdk'

/**
 * Returns the channel options, read fresh so the loaded catalogue answers.
 * @returns The options to offer.
 */
export function channelItems(): { value: string; label: string }[] {
	return [
		{ value: 'email', label: _x('Email', 'contact channel', 'alphone') },
		{ value: 'phone', label: _x('Phone', 'contact channel', 'alphone') },
	]
}

/**
 * Reads the channel item a select item stands for.
 * @param item - The chosen item, or null when the selection is cleared.
 * @returns The matching channel item, or the email item as the default.
 */
export function channelItemOf(item: { value: string | null } | null) {
	const items = channelItems()
	return items.find((candidate) => candidate.value === item?.value) ?? items[0]
}

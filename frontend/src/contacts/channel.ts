// SPDX-License-Identifier: AGPL-3.0-or-later

export const channelItems = [
	{ value: 'email', label: 'Email' },
	{ value: 'phone', label: 'Phone' },
]

/**
 * Reads the channel item a select item stands for.
 * @param item - The chosen item, or null when the selection is cleared.
 * @returns The matching channel item, or the email item as the default.
 */
export function channelItemOf(item: { value: string | null } | null) {
	return channelItems.find((candidate) => candidate.value === item?.value) ?? channelItems[0]
}

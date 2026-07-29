// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * Reads the priority a select item stands for.
 * @param item - The chosen item, or null when the selection is cleared.
 * @returns The priority number.
 */
export function priorityOf(item: { value: string | null } | null): number {
	return Number(item?.value ?? 0)
}

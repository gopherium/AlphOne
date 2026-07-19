// SPDX-License-Identifier: AGPL-3.0-or-later

const createdFormat = new Intl.DateTimeFormat('en-US', {
	month: 'short',
	day: 'numeric',
	year: 'numeric',
})

/**
 * Formats a contact's creation moment for display.
 * @param at - The creation moment.
 * @returns A date such as Jul 6, 2026.
 */
export function formatCreated(at: Date): string {
	return createdFormat.format(at)
}

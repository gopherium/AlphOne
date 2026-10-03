// SPDX-License-Identifier: AGPL-3.0-or-later

import { formatDate } from '@alphone/frontend-sdk'

/**
 * Formats a contact's creation moment for display.
 * @param at - The creation moment.
 * @returns A date such as 06/07/2026.
 */
export function formatCreated(at: Date): string {
	return formatDate(at)
}

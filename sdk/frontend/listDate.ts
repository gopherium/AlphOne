// SPDX-License-Identifier: AGPL-3.0-or-later

import { listDate as writtenListDate, startListDates } from '@gopherium/godmin/dates'
import { displayLocale } from '@gopherium/gottext'

/** started names the interface language the list dates were last started in. */
let started: string | undefined

/**
 * Returns a moment as the abbreviated date and time a list shows, in the interface language.
 * @param at - The moment, as a Date or as the text a server stored.
 * @returns The date and time, such as Oct 13, 2026 5:04 pm.
 */
export function listDate(at: Date | string): string {
	const locale = displayLocale()
	if (started !== locale) {
		startListDates(locale)
		started = locale
	}
	return writtenListDate(at)
}

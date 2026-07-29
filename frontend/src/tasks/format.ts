// SPDX-License-Identifier: AGPL-3.0-or-later

const dayFormat = new Intl.DateTimeFormat('en-US', {
	weekday: 'long',
	month: 'short',
	day: 'numeric',
})

/**
 * Formats a date as the YYYY-MM-DD the task API expects, in local time.
 * @param at - The date to format.
 * @returns A date such as 2026-07-30.
 */
export function isoDate(at: Date): string {
	const year = String(at.getFullYear()).padStart(4, '0')
	const month = String(at.getMonth() + 1).padStart(2, '0')
	const day = String(at.getDate()).padStart(2, '0')
	return `${year}-${month}-${day}`
}

/**
 * Formats a task due date for the day heading.
 * @param date - The due date as YYYY-MM-DD.
 * @returns A heading such as Thursday, Jul 30.
 */
export function formatDay(date: string): string {
	const [year, month, day] = date.split('-').map(Number)
	return dayFormat.format(new Date(year, month - 1, day))
}

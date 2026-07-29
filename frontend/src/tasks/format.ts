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
	return dayFormat.format(parseDate(date))
}

/**
 * Reports whether a string is a calendar date the task API accepts.
 * @param date - The candidate date.
 * @returns True when the string is a real YYYY-MM-DD date.
 */
export function isValidDate(date: string): boolean {
	return /^\d{4}-\d{2}-\d{2}$/.test(date) && isoDate(parseDate(date)) === date
}

/**
 * Shifts a date by whole days.
 * @param date - The starting date as YYYY-MM-DD.
 * @param days - The number of days to add.
 * @returns The shifted date as YYYY-MM-DD.
 */
export function shiftDate(date: string, days: number): string {
	const at = parseDate(date)
	at.setDate(at.getDate() + days)
	return isoDate(at)
}

/**
 * Returns the later of two dates.
 * @param date - The first date as YYYY-MM-DD.
 * @param other - The second date as YYYY-MM-DD.
 * @returns The later date as YYYY-MM-DD.
 */
export function laterDate(date: string, other: string): string {
	return date > other ? date : other
}

/**
 * Parses a task date as a local calendar day.
 * @param date - The date as YYYY-MM-DD.
 * @returns The parsed date at local midnight.
 */
function parseDate(date: string): Date {
	const [year, month, day] = date.split('-').map(Number)
	return new Date(year, month - 1, day)
}

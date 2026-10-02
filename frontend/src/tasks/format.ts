// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, _x, formatDate, formatWeekday, sprintf } from '@alphone/frontend-sdk'

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
 * @returns A heading such as Thursday, 30/07/2026.
 */
export function formatDay(date: string): string {
	return sprintf(_x('%(weekday)s, %(date)s', 'day heading', 'alphone'), {
		weekday: formatWeekday(date),
		date: formatDate(date),
	})
}

/**
 * Formats a task due date for a row.
 * @param date - The due date as YYYY-MM-DD.
 * @returns A label such as Due 30/07/2026.
 */
export function formatDue(date: string): string {
	return sprintf(__('Due %(date)s', 'alphone'), { date: formatDate(date) })
}

/**
 * Returns the toast a task list raises once a task moved to a new day.
 * @param date - The new due date as YYYY-MM-DD.
 * @returns A message such as Task moved to 30/07/2026.
 */
export function movedMessage(date: string): string {
	return sprintf(__('Task moved to %(date)s.', 'alphone'), { date: formatDate(date) })
}

/**
 * Returns the toast raised once a task took a new status.
 * @param status - The status the task took.
 * @returns Task completed. for a done task, Task reopened. for an open one.
 */
export function statusMessage(status: string): string {
	return status === 'done' ? __('Task completed.', 'alphone') : __('Task reopened.', 'alphone')
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

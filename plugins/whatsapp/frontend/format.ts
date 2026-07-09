// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * Formats a moment as its local calendar date.
 * @param at - The moment to format.
 * @returns The date in YYYY-MM-DD form.
 */
export function formatDay(at: Date): string {
	const year = at.getFullYear()
	const month = String(at.getMonth() + 1).padStart(2, '0')
	const day = String(at.getDate()).padStart(2, '0')
	return `${year}-${month}-${day}`
}

/**
 * Formats a moment as a zero-padded 24-hour local clock time.
 * @param at - The moment to format.
 * @returns The time in HH:MM form.
 */
export function formatTime(at: Date): string {
	const hours = String(at.getHours()).padStart(2, '0')
	const minutes = String(at.getMinutes()).padStart(2, '0')
	return `${hours}:${minutes}`
}

const dayLabelFormat = new Intl.DateTimeFormat('en-US', {
	month: 'short',
	day: 'numeric',
	year: 'numeric',
})

/**
 * Labels a moment's calendar day for display, relative to the current moment.
 * @param at - The moment to label.
 * @param now - The current moment, anchoring Today and Yesterday.
 * @returns Today, Yesterday, or a date such as Jul 6, 2026.
 */
export function formatDayLabel(at: Date, now: Date): string {
	if (formatDay(at) === formatDay(now)) {
		return 'Today'
	}
	const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1)
	if (formatDay(at) === formatDay(yesterday)) {
		return 'Yesterday'
	}
	return dayLabelFormat.format(at)
}

/**
 * Formats a conversation's last activity for its list row: the clock time when
 * the activity happened today, the labelled day otherwise.
 * @param at - The moment of the last activity.
 * @param now - The current moment, deciding whether the activity is today's.
 * @returns The time in HH:MM form for today's activity, else the day label.
 */
export function formatListTime(at: Date, now: Date): string {
	if (formatDay(at) === formatDay(now)) {
		return formatTime(at)
	}
	return formatDayLabel(at, now)
}

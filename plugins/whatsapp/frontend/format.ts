// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, formatDate, formatNumber, formatTime, sprintf } from '@alphone/frontend-sdk'

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
 * Labels a moment's calendar day for display, relative to the current moment.
 * @param at - The moment to label.
 * @param now - The current moment, anchoring Today and Yesterday.
 * @returns Today, Yesterday, or a date such as 06/07/2026.
 */
export function formatDayLabel(at: Date, now: Date): string {
	if (formatDay(at) === formatDay(now)) {
		return __('Today', 'alphone-whatsapp')
	}
	const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1)
	if (formatDay(at) === formatDay(yesterday)) {
		return __('Yesterday', 'alphone-whatsapp')
	}
	return formatDate(at)
}

/**
 * Formats a byte count as a compact human readable size.
 * @param bytes - The size in bytes.
 * @returns The size label in B, KB, or MB.
 */
export function formatFileSize(bytes: number): string {
	if (bytes < 1024) {
		return sprintf(__('%(size)s B', 'alphone-whatsapp'), { size: formatNumber(bytes) })
	}
	if (bytes < 1024 * 1024) {
		return sprintf(__('%(size)s KB', 'alphone-whatsapp'), { size: formatNumber(Math.round(bytes / 1024)) })
	}
	const megabytes = formatNumber(bytes / (1024 * 1024), { minimumFractionDigits: 1, maximumFractionDigits: 1 })
	return sprintf(__('%(size)s MB', 'alphone-whatsapp'), { size: megabytes })
}

/**
 * Formats a conversation's last activity for its list row: the clock time when
 * the activity happened today, the labelled day otherwise.
 * @param at - The moment of the last activity.
 * @param now - The current moment, deciding whether the activity is today's.
 * @returns The time, such as 09:05, for today's activity, else the day label.
 */
export function formatListTime(at: Date, now: Date): string {
	if (formatDay(at) === formatDay(now)) {
		return formatTime(at)
	}
	return formatDayLabel(at, now)
}

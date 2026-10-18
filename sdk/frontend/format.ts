// SPDX-License-Identifier: AGPL-3.0-or-later

import { displayLocale } from '@gopherium/gottext'

/** named holds the locale the server named for dates, times, numbers and money. */
let named: string | undefined

/** CALENDAR_DAY is the shape of a bare calendar day, such as 2026-09-30. */
const CALENDAR_DAY = /^\d{4}-\d{2}-\d{2}$/

/** WALL_CLOCK is the shape of a bare wall clock time, such as 09:30. */
const WALL_CLOCK = /^\d{2}:\d{2}$/

/** DAY is how a date is written. */
const DAY: Intl.DateTimeFormatOptions = { day: '2-digit', month: '2-digit', year: 'numeric' }

/** CLOCK is how a time is written. */
const CLOCK: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }

/**
 * Stores the locale dates, times, numbers and money are written in.
 * @param locale - The locale the server named, or undefined to forget it.
 */
export function rememberFormatLocale(locale: string | undefined): void {
	named = locale
}

/**
 * Returns the locale dates, times, numbers and money are written in.
 * @returns The locale the server named, the interface locale until it names one.
 */
function formatLocale(): string {
	return named ?? displayLocale()
}

/**
 * Returns a moment and the options it is written with, a bare calendar day or wall clock time kept on what it names.
 * @param at - The moment, or the text a server stored.
 * @param options - How the moment is written.
 * @returns The moment as a date beside the options to write it with.
 */
function moment(at: Date | string, options: Intl.DateTimeFormatOptions): [Date, Intl.DateTimeFormatOptions] {
	if (typeof at === 'string' && WALL_CLOCK.test(at)) {
		return [new Date(`1970-01-01T${at}:00Z`), { ...options, timeZone: 'UTC' }]
	}
	const day = typeof at === 'string' && CALENDAR_DAY.test(at)
	return [new Date(at), day ? { ...options, timeZone: 'UTC' } : options]
}

/**
 * Returns a moment as a date in the format locale.
 * @param at - The moment, the text a server stored, or a bare calendar day.
 * @returns The date, such as 30/09/2026.
 */
export function formatDate(at: Date | string): string {
	const [shown, options] = moment(at, DAY)
	return shown.toLocaleDateString(formatLocale(), options)
}

/**
 * Returns a moment as a time of day in the format locale.
 * @param at - The moment, the text a server stored, or a bare wall clock time.
 * @returns The time, such as 09:05.
 */
export function formatTime(at: Date | string): string {
	const [shown, options] = moment(at, CLOCK)
	return shown.toLocaleTimeString(formatLocale(), options)
}

/**
 * Returns the name of the weekday a moment falls on, in the interface language.
 * @param at - The moment, the text a server stored, or a bare calendar day.
 * @returns The weekday, such as Wednesday.
 */
export function formatWeekday(at: Date | string): string {
	const [shown, options] = moment(at, { weekday: 'long' })
	return shown.toLocaleDateString(displayLocale(), options)
}

/**
 * Returns a number in the format locale, its thousands always grouped.
 * @param value - The number.
 * @param options - How many decimals and which style to write it with.
 * @returns The number, such as 1.234,56.
 */
export function formatNumber(value: number, options: Intl.NumberFormatOptions = {}): string {
	return new Intl.NumberFormat(formatLocale(), { ...options, useGrouping: 'always' }).format(value)
}

/**
 * Returns the items as one list in the interface language.
 * @param items - The words to list, in order.
 * @returns The list, such as Birth date, Shoe size.
 */
export function formatList(items: readonly string[]): string {
	return new Intl.ListFormat(displayLocale(), { type: 'unit' }).format(items)
}

/**
 * Returns an amount of money in the format locale.
 * @param amount - The amount.
 * @param currency - The ISO 4217 code of the currency the amount is in.
 * @returns The amount, such as 1.234,56 €.
 */
export function formatMoney(amount: number, currency: string): string {
	return formatNumber(amount, { style: 'currency', currency })
}

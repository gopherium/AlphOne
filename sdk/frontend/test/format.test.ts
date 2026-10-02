// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberLocale } from '@gopherium/gottext'
import { resetLocale } from '@gopherium/gottext/testing'
import { afterEach, expect, test, vi } from 'vitest'

import {
	formatDate,
	formatList,
	formatMoney,
	formatNumber,
	formatTime,
	formatWeekday,
	rememberFormatLocale,
} from '../index'

afterEach(() => {
	vi.unstubAllEnvs()
	rememberFormatLocale(undefined)
	resetLocale()
})

test('writes a date day first with a two digit month and a four digit year', () => {
	rememberFormatLocale('es-ES')

	expect(formatDate(new Date(2026, 8, 30, 23, 30))).toBe('30/09/2026')
	expect(formatDate(new Date(2026, 6, 6))).toBe('06/07/2026')
})

test('writes a moment a server stored on the local day it fell', () => {
	rememberFormatLocale('es-ES')

	expect(formatDate('2026-09-30T09:05:00Z')).toBe('30/09/2026')
})

test('writes a bare calendar day on the day it names in every time zone', () => {
	rememberFormatLocale('es-ES')
	vi.stubEnv('TZ', 'America/New_York')

	expect(formatDate('2026-09-30')).toBe('30/09/2026')
})

test('writes a time on a 24 hour clock with hours and minutes', () => {
	rememberFormatLocale('es-ES')

	expect(formatTime(new Date(2026, 8, 30, 21, 5))).toBe('21:05')
	expect(formatTime('2026-09-30T09:05:00Z')).toBe('09:05')
})

test('groups the thousands of every number with a dot and writes decimals after a comma', () => {
	rememberFormatLocale('es-ES')

	expect(formatNumber(1234)).toBe('1.234')
	expect(formatNumber(1234.56)).toBe('1.234,56')
	expect(formatNumber(1234567)).toBe('1.234.567')
	expect(formatNumber(999)).toBe('999')
})

test('writes a number to the precision a screen asks for', () => {
	rememberFormatLocale('es-ES')

	expect(formatNumber(1.55, { maximumFractionDigits: 1 })).toBe('1,6')
})

test('writes an amount of euros after its number', () => {
	rememberFormatLocale('es-ES')

	expect(formatMoney(1234.56, 'EUR')).toBe('1.234,56 €')
})

test('keeps the format locale whatever language the interface stands in', () => {
	rememberLocale('en-US')
	rememberFormatLocale('es-ES')

	expect(formatDate(new Date(2026, 8, 30))).toBe('30/09/2026')
	expect(formatNumber(1234.56)).toBe('1.234,56')
})

test('writes the conventions of the locale the server names', () => {
	rememberFormatLocale('de-DE')

	expect(formatDate(new Date(2026, 8, 30))).toBe('30.09.2026')

	rememberFormatLocale('en-GB')

	expect(formatNumber(1234.5)).toBe('1,234.5')
})

test('writes in the interface locale until the server names a format locale', () => {
	rememberFormatLocale(undefined)
	rememberLocale('en-US')

	expect(formatDate(new Date(2026, 8, 30))).toBe('09/30/2026')
})

test('names a weekday in the interface language whatever the format locale', () => {
	rememberFormatLocale('en-GB')
	rememberLocale('es-ES')

	expect(formatWeekday('2026-09-30')).toBe('miércoles')

	rememberLocale('en-US')

	expect(formatWeekday(new Date(2026, 8, 30))).toBe('Wednesday')
})

test('joins a list the way the interface language writes one', () => {
	rememberFormatLocale('de-DE')
	rememberLocale('es-ES')

	expect(formatList(['Fecha de nacimiento', 'Talla de calzado'])).toBe('Fecha de nacimiento y Talla de calzado')

	rememberLocale('en-US')

	expect(formatList(['Birth date', 'Shoe size', 'Height'])).toBe('Birth date, Shoe size, Height')
})

test('names the weekday a bare calendar day falls on in every time zone', () => {
	rememberLocale('en-US')
	vi.stubEnv('TZ', 'America/New_York')

	expect(formatWeekday('2026-09-30')).toBe('Wednesday')
})

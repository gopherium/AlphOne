// SPDX-License-Identifier: AGPL-3.0-or-later

import { LIST_CHROME_DOMAIN, listChromeCatalogFor } from '@gopherium/godmin'
import { rememberLocale } from '@gopherium/gottext'
import { resetLocale } from '@gopherium/gottext/testing'
import { resetLocaleData, setLocaleData } from '@wordpress/i18n'
import { afterEach, expect, test } from 'vitest'

import { listDate } from '../dataviews'

/** Settles the interface on Spanish with the list catalogue loaded. */
async function readInSpanish(): Promise<void> {
	setLocaleData(await listChromeCatalogFor('es-ES'), LIST_CHROME_DOMAIN)
	rememberLocale('es-ES')
}

afterEach(() => {
	resetLocaleData(undefined, LIST_CHROME_DOMAIN)
	resetLocale()
})

test('writes a moment a server stored as the date and time a list shows', () => {
	expect(listDate('2026-10-13T15:04:00Z')).toBe('Oct 13, 2026 3:04 pm')
})

test('writes a moment handed as a date the same way', () => {
	expect(listDate(new Date('2026-10-13T15:04:00Z'))).toBe('Oct 13, 2026 3:04 pm')
})

test('writes list dates in the language the reader settled on', async () => {
	await readInSpanish()

	expect(listDate('2026-08-03T09:05:00Z')).toBe('3 Ago 2026 09:05')
})

test('writes list dates again in the language a later boot settles on', async () => {
	expect(listDate('2026-08-03T09:05:00Z')).toBe('Aug 3, 2026 9:05 am')
	await readInSpanish()

	expect(listDate('2026-08-03T09:05:00Z')).toBe('3 Ago 2026 09:05')
})

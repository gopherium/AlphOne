// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { afterEach, expect, test } from 'vitest'

import { coreNav } from '../coreNav'
import { startAppLocale } from '../../i18n/start'

afterEach(() => {
	resetLocale()
})

/** labelsIn settles the interface on one locale and reads every core menu entry. */
async function labelsIn(locale: string): Promise<string[]> {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale } })))
	await startAppLocale()
	return coreNav.map((item) => item.label)
}

test('reads every core menu entry in the language the interface stands in', async () => {
	const english = await labelsIn('en-US')
	resetLocale()
	const spanish = await labelsIn('es-ES')

	const frozen = coreNav
		.filter((_, at) => spanish[at] === english[at])
		.map((item) => item.to)

	expect(frozen).toEqual([])
})

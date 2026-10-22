// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { afterEach, expect, test } from 'vitest'

import { coreNav, shownInstead } from '../coreNav'
import { startAppLocale } from '../../i18n/start'

/** entries is every core menu entry and every entry shown in place of one. */
const entries = [...coreNav, ...shownInstead.values()]

afterEach(() => {
	resetLocale()
})

/** labelsIn settles the interface on one locale and reads every entry. */
async function labelsIn(locale: string): Promise<string[]> {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale } })))
	await startAppLocale()
	return entries.map((item) => item.label)
}

test('reads every core menu entry and every entry shown in place of one in the interface language', async () => {
	const english = await labelsIn('en-US')
	resetLocale()
	const spanish = await labelsIn('es-ES')

	const frozen = entries
		.filter((_, at) => spanish[at] === english[at])
		.map((item) => item.to)

	expect(frozen).toEqual([])
})

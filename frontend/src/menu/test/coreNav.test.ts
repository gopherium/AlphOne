// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { usersMenuItem } from '@gopherium/react-auth/wpds'
import { afterEach, expect, test } from 'vitest'

import { coreNav } from '../coreNav'
import { startAppLocale } from '../../i18n/start'

/** entries is every core menu row and every row shown in place of one. */
const entries = [...coreNav, ...coreNav.flatMap((item) => (item.fallback === undefined ? [] : [item.fallback]))]

afterEach(() => {
	resetLocale()
})

/** labelsIn settles the interface on one locale and reads every row. */
async function labelsIn(locale: string): Promise<string[]> {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale } })))
	await startAppLocale()
	return entries.map((item) => item.label)
}

test('reads every core menu row and every row shown in place of one in the interface language', async () => {
	const english = await labelsIn('en-US')
	resetLocale()
	const spanish = await labelsIn('es-ES')

	const frozen = entries
		.filter((_, at) => spanish[at] === english[at])
		.map((item) => item.to)

	expect(frozen).toEqual([])
})

test('offers the reader who cannot open Users their own API tokens in its place, under the Users glyph', () => {
	const users = coreNav.find((item) => item.to === usersMenuItem.to)

	expect(users?.fallback).toMatchObject({ to: '/users/tokens', icon: usersMenuItem.icon })
})

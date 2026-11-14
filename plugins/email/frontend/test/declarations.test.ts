// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { DOMAIN, plugin } from '../index'

test('grants the area its mail is held to, named in English until a catalogue loads', () => {
	expect(plugin.tokenAreas).toEqual([{ value: 'email', label: 'email' }])
})

test('reads the label of its area fresh on every use', () => {
	const area = plugin.tokenAreas?.[0]

	expect(area && Object.getOwnPropertyDescriptor(area, 'label')?.get).toBeTypeOf('function')
})

test('names its area in Spanish in its own catalogue', async () => {
	const catalog = await plugin.locale?.load('es-ES')

	expect((catalog?.['token area\u0004email'] as string[] | undefined)?.[0]).toBe('correo electrónico')
})

test('answers its strings under its own text domain', () => {
	expect(DOMAIN).toBe('alphone-email')
	expect(plugin.locale?.domain).toBe(DOMAIN)
})

test('adds no screen and no menu item of its own', () => {
	expect(plugin.nav).toEqual([])
	expect(plugin.routes(undefined as never)).toEqual([])
})

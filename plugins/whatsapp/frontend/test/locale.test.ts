// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { plugin } from '../index'

test('declares its own text domain', () => {
	expect(plugin.locale?.domain).toBe('alphone-whatsapp')
})

test('answers the catalogue for the locale it ships', async () => {
	expect(await plugin.locale?.load('es-ES')).toBeDefined()
})

test('answers no catalogue for a locale shipping none', async () => {
	expect(await plugin.locale?.load('xx-XX')).toBeUndefined()
})

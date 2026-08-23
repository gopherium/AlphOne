// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { plugin } from '../index'

test('declares its own text domain', () => {
	expect(plugin.locale?.domain).toBe('alphone-fields')
})

test('answers no catalogue for a locale shipping none', async () => {
	expect(await plugin.locale?.load('xx-XX')).toBeUndefined()
})

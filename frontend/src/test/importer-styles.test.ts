// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { declarations } from './stylesheet'

test('lets an import row reason wrap in lines no narrower than 24 characters', () => {
	expect(declarations('.alphone-import__reason')).toEqual({ 'white-space': 'normal', 'min-width': '24ch' })
})

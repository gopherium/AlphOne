// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { declarations } from './stylesheet'

test('gives the read and write boxes of an area equal halves of their row', () => {
	expect(declarations('.alphone-tokens__grant')).toEqual({ flex: '1 1 0', 'min-width': '0' })
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { stringifyDocument } from 'urql'
import { expect, test } from 'vitest'

import { printGraphDocument } from '../index'

test('hands the host the printer urql sends every document with', () => {
	expect(printGraphDocument).toBe(stringifyDocument)
})

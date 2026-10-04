// SPDX-License-Identifier: AGPL-3.0-or-later

import { close as wordpressClose, moveTo as wordpressMoveTo } from '@wordpress/icons'
import { expect, test } from 'vitest'

import { close, moveTo } from '../index'

test('hands plugins the move and close icons a bulk action draws', () => {
	expect(moveTo).toBe(wordpressMoveTo)
	expect(close).toBe(wordpressClose)
})

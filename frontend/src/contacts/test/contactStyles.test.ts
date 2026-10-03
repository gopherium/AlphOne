// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { declarations } from '../../test/stylesheet'

test('keeps a channel badge on one line, over the wrapping the design system text allows', () => {
	expect(declarations('.alphone-contacts__channel')).toEqual({ 'white-space': 'nowrap' })
})

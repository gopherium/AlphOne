// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { plugin } from '../index'

test('names the channel its identities arrive on', () => {
	expect(plugin.channels).toEqual([{ value: 'whatsapp', label: 'WhatsApp' }])
})

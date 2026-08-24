// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { channelItemOf, channelItems } from '../contacts/channel'

test('reads the channel item a selection stands for', () => {
	expect(channelItemOf({ value: 'phone' })).toEqual({ value: 'phone', label: 'Phone' })
	expect(channelItemOf({ value: 'email' })).toEqual({ value: 'email', label: 'Email' })
})

test('falls back to email without a selection', () => {
	expect(channelItemOf(null)).toEqual(channelItems()[0])
	expect(channelItemOf({ value: null })).toEqual(channelItems()[0])
})

import { expect, test } from 'vitest'

import { channelItemOf, channelItems } from '../contacts/channel'

test('reads the channel item a selection stands for', () => {
	expect(channelItemOf({ value: 'phone' })).toEqual({ value: 'phone', label: 'Phone' })
	expect(channelItemOf({ value: 'email' })).toEqual({ value: 'email', label: 'Email' })
})

test('falls back to email without a selection', () => {
	expect(channelItemOf(null)).toBe(channelItems[0])
	expect(channelItemOf({ value: null })).toBe(channelItems[0])
})

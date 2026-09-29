// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import type { ChannelName, FrontendPlugin } from '../index'

test('a plugin may name no channels', () => {
	const plugin: FrontendPlugin = { id: 'quiet', routes: () => [], nav: [] }

	expect(plugin.channels).toBeUndefined()
})

test('a plugin names the channels its identities arrive on', () => {
	const channel: ChannelName = { value: 'chat', label: 'Chat' }
	const plugin: FrontendPlugin = { id: 'chat', routes: () => [], nav: [], channels: [channel] }

	expect(plugin.channels).toEqual([{ value: 'chat', label: 'Chat' }])
})

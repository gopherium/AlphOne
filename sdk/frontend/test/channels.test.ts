// SPDX-License-Identifier: AGPL-3.0-or-later

import { afterEach, expect, test, vi } from 'vitest'

import { channelName, configureChannelNames } from '../index'
import type { ChannelName, FrontendPlugin } from '../index'

afterEach(() => {
	configureChannelNames(() => [])
})

test('a plugin may name no channels', () => {
	const plugin: FrontendPlugin = { id: 'quiet', routes: () => [], nav: [] }

	expect(plugin.channels).toBeUndefined()
})

test('a plugin names the channels its identities arrive on', () => {
	const channel: ChannelName = { value: 'chat', label: 'Chat' }
	const plugin: FrontendPlugin = { id: 'chat', routes: () => [], nav: [], channels: [channel] }

	expect(plugin.channels).toEqual([{ value: 'chat', label: 'Chat' }])
})

test('a plugin reads the name the core or another plugin gives a channel', () => {
	configureChannelNames(() => [
		{ value: 'email', label: 'Email' },
		{ value: 'chat', label: 'Chat' },
	])

	expect(channelName('chat')).toBe('Chat')
})

test('a plugin reads a channel nobody names as the channel itself', () => {
	configureChannelNames(() => [{ value: 'email', label: 'Email' }])

	expect(channelName('fax')).toBe('fax')
})

test('a plugin reads a channel as itself before the core hands over any names', async () => {
	vi.resetModules()
	const fresh = await import('../channels')

	expect(fresh.channelName('chat')).toBe('chat')
})

test('a plugin reads the name in the catalogue loaded at the time it reads', () => {
	let label = 'Chat'
	configureChannelNames(() => [{ value: 'chat', label }])

	label = 'Charla'

	expect(channelName('chat')).toBe('Charla')
})

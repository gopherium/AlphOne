// SPDX-License-Identifier: AGPL-3.0-or-later

import { envelope } from '@alphone/frontend-sdk'
import { fakeGraphClient, server } from '@alphone/frontend-sdk/testing'
import { createRootRoute } from '@tanstack/react-router'
import { expect, test, vi } from 'vitest'

import { handlers } from '../handlers'
import { DOMAIN, plugin } from '../index'
import { inboxQuery, mailboxQuery } from '../operations'

test('grants the area its mail is held to, named in English until a catalogue loads', () => {
	expect(plugin.tokenAreas).toEqual([{ value: 'email', label: 'email' }])
})

test('reads the label of its area fresh on every use', () => {
	const area = plugin.tokenAreas?.[0]

	expect(area && Object.getOwnPropertyDescriptor(area, 'label')?.get).toBeTypeOf('function')
})

test('names its area in Spanish in its own catalogue', async () => {
	const catalog = await plugin.locale?.load('es-ES')

	expect((catalog?.['token area\u0004email'] as string[] | undefined)?.[0]).toBe('correo electrónico')
})

test('answers its strings under its own text domain', () => {
	expect(DOMAIN).toBe('alphone-email')
	expect(plugin.locale?.domain).toBe(DOMAIN)
})

test('adds one Email row to the menu, leading to the inbox under the envelope glyph', () => {
	expect(plugin.nav).toEqual([{ label: 'Email', to: '/email', icon: envelope }])
})

test('reads the label of its menu row fresh on every use', () => {
	const row = plugin.nav[0]

	expect(row && Object.getOwnPropertyDescriptor(row, 'label')?.get).toBeTypeOf('function')
})

test('mounts the inbox at the address its menu row leads to', () => {
	const paths = plugin.routes(createRootRoute()).map((route) => (route.options as { path?: string }).path)

	expect(paths).toEqual(['/email'])
})

test('keys the seven types that carry no id, so the cache keeps each inside its parent', () => {
	const keys = plugin.graphKeys ?? {}

	expect(Object.keys(keys)).toEqual([
		'EmailThreadPage',
		'EmailMessagePage',
		'EmailAddress',
		'EmailParticipant',
		'EmailBody',
		'EmailUsage',
		'EmailDeletion',
	])
	expect(Object.values(keys).map((key) => key({}))).toEqual([null, null, null, null, null, null, null])
})

test('a client keying the plugin caches a page of threads and their senders without warning', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	server.use(...handlers)
	const { graph } = fakeGraphClient([plugin])

	const result = await graph.client.query(inboxQuery, { limit: 2, offset: 0 }).toPromise()

	expect(result.data?.emailThreads.items.map((thread) => thread.latest.from?.name)).toEqual([
		'Ada Lovelace',
		'Maria Perez',
	])
	expect(warn).not.toHaveBeenCalled()
})

test('a client keying the plugin caches the mailbox without warning', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	server.use(...handlers)
	const { graph } = fakeGraphClient([plugin])

	const result = await graph.client.query(mailboxQuery, {}).toPromise()

	expect(result.data?.emailMailbox?.address).toBe('info@example.com')
	expect(warn).not.toHaveBeenCalled()
})

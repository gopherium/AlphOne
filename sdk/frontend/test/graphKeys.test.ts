// SPDX-License-Identifier: AGPL-3.0-or-later

import { gql } from 'urql'
import { expect, test, vi } from 'vitest'

import { createGraphClient, graphKeysOf } from '../graph'
import type { FrontendPlugin } from '../index'
import { HttpResponse, fakeGraphClient, graphql, server } from '../testing'

/** GraphKeyedPlugins is the plugins a client keys the types of. */
type GraphKeyedPlugins = Pick<FrontendPlugin, 'id' | 'graphKeys'>[]

/** probeQuery reads one embedded value of a type the test names, a type without an id. */
const probeQuery = gql`
	query Probe {
		probe {
			value
		}
	}
`

/**
 * Serves the probe query with a value of the named type.
 * @param typename - The type the embedded value carries.
 */
function serveProbe(typename: string) {
	server.use(
		graphql.query('Probe', () => HttpResponse.json({ data: { probe: { __typename: typename, value: 'x' } } })),
	)
}

test('keys the core types the graph returns without an id, and only those', () => {
	const keys = graphKeysOf([])

	expect(Object.keys(keys).sort()).toEqual([
		'AdminSettings',
		'ContactPage',
		'CreateTaskPayload',
		'CreateWebhookPayload',
		'LoginPayload',
	])
	for (const key of Object.values(keys)) {
		expect(key({})).toBeNull()
	}
})

test('adds the keys each plugin declares beside the core ones', () => {
	const keys = graphKeysOf([
		{ id: 'alpha', graphKeys: { AlphaEmbed: () => null } },
		{ id: 'beta', graphKeys: { BetaNamed: (data) => String(data.name) } },
		{ id: 'gamma' },
	])

	expect(Object.keys(keys)).toEqual(expect.arrayContaining(['AdminSettings', 'AlphaEmbed', 'BetaNamed']))
	expect(keys.BetaNamed({ name: 'date' })).toBe('date')
})

test('refuses a type two plugins key, naming both', () => {
	expect(() =>
		graphKeysOf([
			{ id: 'alpha', graphKeys: { SharedEmbed: () => null } },
			{ id: 'beta', graphKeys: { SharedEmbed: () => null } },
		]),
	).toThrow('graph: beta keys SharedEmbed, which alpha already keys')
})

test('refuses a plugin keying a type the core keys', () => {
	expect(() => graphKeysOf([{ id: 'alpha', graphKeys: { AdminSettings: () => null } }])).toThrow(
		'graph: alpha keys AdminSettings, which the core already keys',
	)
})

test('answers a fresh map, leaving the core keys as they were', () => {
	graphKeysOf([{ id: 'alpha', graphKeys: { LeakProbe: () => null } }])

	expect(graphKeysOf([])).not.toHaveProperty('LeakProbe')
})

test.each([
	{
		client: 'a graph client',
		typename: 'ClientKeyed',
		build: (plugins: GraphKeyedPlugins) => createGraphClient({ onSessionExpired: vi.fn(), plugins }),
	},
	{
		client: 'a fake graph client',
		typename: 'FakeKeyed',
		build: (plugins: GraphKeyedPlugins) => fakeGraphClient(plugins).graph,
	},
])('$client caches the id-less type a plugin keys without warning', async ({ typename, build }) => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	serveProbe(typename)
	const graph = build([{ id: 'probe', graphKeys: { [typename]: () => null } }])

	const result = await graph.client.query(probeQuery, {}).toPromise()

	expect(result.error).toBeUndefined()
	expect(warn).not.toHaveBeenCalled()
})

test('a graph client warns about an id-less type no plugin keys', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	serveProbe('ClientUnkeyed')
	const graph = createGraphClient({ onSessionExpired: vi.fn() })

	await graph.client.query(probeQuery, {}).toPromise()

	expect(warn).toHaveBeenCalledWith(expect.stringContaining('ClientUnkeyed'))
})

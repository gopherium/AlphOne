// SPDX-License-Identifier: AGPL-3.0-or-later

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render } from '@testing-library/react'
import { Provider } from 'urql'
import { expect, test } from 'vitest'

import { useGraphStream } from '../stream'
import { fakeGraphClient } from '../testing'
import type { FakeGraph } from '../testing'

/** Renders a probe subscribing to the core events, settling its subscription. */
async function renderProbe(fake: FakeGraph, operations: readonly string[]) {
	function Probe() {
		useGraphStream({ graph: fake.graph, operations })
		return null
	}
	const client = new QueryClient()
	const tree = (
		<QueryClientProvider client={client}>
			<Provider value={fake.graph.client}>
				<Probe />
			</Provider>
		</QueryClientProvider>
	)
	const view = render(tree)
	await act(async () => {})
	return { view, client, tree }
}

test('rings the doorbell for the named operations on every frame', async () => {
	const fake = fakeGraphClient()
	await renderProbe(fake, ['Tasks', 'Contacts'])

	fake.emit({ coreEvent: 'task.created' })

	expect(fake.graph.refetch).toHaveBeenCalledWith(['Tasks', 'Contacts'])
})

test('subscribes to the core events rather than a plugin stream', async () => {
	const fake = fakeGraphClient()

	await renderProbe(fake, ['Tasks'])

	expect(fake.documents[0]).toContain('coreEvent')
})

test('rings the doorbell once the stream connects so a reconnect catches up', async () => {
	const fake = fakeGraphClient()
	await renderProbe(fake, ['Tasks'])

	fake.openStream()

	expect(fake.graph.refetch).toHaveBeenCalledWith(['Tasks'])
})

test('unsubscribes when the subscriber unmounts', async () => {
	const fake = fakeGraphClient()
	const { view } = await renderProbe(fake, ['Tasks'])

	view.unmount()

	expect(fake.unsubscribes()).toBe(1)
})

test('keeps one subscription when the operations array is rebuilt each render', async () => {
	const fake = fakeGraphClient()
	function Probe({ operations }: { operations: readonly string[] }) {
		useGraphStream({ graph: fake.graph, operations })
		return null
	}
	const client = new QueryClient()
	const probeAt = (operations: readonly string[]) => (
		<QueryClientProvider client={client}>
			<Provider value={fake.graph.client}>
				<Probe operations={operations} />
			</Provider>
		</QueryClientProvider>
	)
	const view = render(probeAt(['Tasks']))
	await act(async () => {})

	view.rerender(probeAt(['Tasks']))
	await act(async () => {})

	expect(fake.documents).toHaveLength(1)
})

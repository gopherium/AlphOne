// SPDX-License-Identifier: AGPL-3.0-or-later

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import type { Client } from 'urql'
import { afterEach, expect, test, vi } from 'vitest'

import type { GraphClient } from '../graph'
import { useGraphStream } from '../stream'
import { FakeEventSource } from '../testing'

afterEach(() => {
	vi.restoreAllMocks()
})

/** Returns a graph client whose doorbell rings are observable. */
function fakeGraph() {
	const refetch = vi.fn<(operations: readonly string[]) => void>()
	const graph: GraphClient = { client: {} as Client, refetch }
	return { graph, refetch }
}

/** Renders a probe subscribing to the stream for the given operations. */
function renderProbe(graph: GraphClient, operations: readonly string[]) {
	function Probe() {
		useGraphStream('/stream', { graph, operations })
		return null
	}
	const client = new QueryClient()
	return render(
		<QueryClientProvider client={client}>
			<Probe />
		</QueryClientProvider>,
	)
}

test('rings the doorbell for the named operations on every event', () => {
	const { graph, refetch } = fakeGraph()
	renderProbe(graph, ['Tasks', 'Contacts'])

	FakeEventSource.last().emit()

	expect(refetch).toHaveBeenCalledWith(['Tasks', 'Contacts'])
})

test('rings the doorbell once the stream opens so a reconnect catches up', () => {
	const { graph, refetch } = fakeGraph()
	renderProbe(graph, ['Tasks'])

	FakeEventSource.last().emitOpen()

	expect(refetch).toHaveBeenCalledWith(['Tasks'])
})

test('closes the stream when the subscriber unmounts', () => {
	const { graph } = fakeGraph()
	const view = renderProbe(graph, ['Tasks'])
	const source = FakeEventSource.last()

	view.unmount()

	expect(source.closed).toBe(true)
})

test('keeps one connection when the operations array is rebuilt each render', () => {
	const { graph } = fakeGraph()
	function Probe() {
		useGraphStream('/stream', { graph, operations: ['Tasks'] })
		return null
	}
	const client = new QueryClient()
	const view = render(
		<QueryClientProvider client={client}>
			<Probe />
		</QueryClientProvider>,
	)

	view.rerender(
		<QueryClientProvider client={client}>
			<Probe />
		</QueryClientProvider>,
	)

	expect(FakeEventSource.instances).toHaveLength(1)
})

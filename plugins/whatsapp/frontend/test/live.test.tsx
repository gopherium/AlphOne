// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider } from '@alphone/frontend-sdk'
import { fakeGraphClient } from '@alphone/frontend-sdk/testing'
import type { FakeGraph } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { useLiveUpdates } from '../live'

/** Renders a probe running the plugin's live updates, settling its subscription. */
async function renderProbe(fake: FakeGraph) {
	function Probe() {
		useLiveUpdates()
		return null
	}
	const client = new QueryClient()
	const view = render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={fake.graph}>
				<Probe />
			</GraphProvider>
		</QueryClientProvider>,
	)
	await act(async () => {})
	return { client, view }
}

test('invalidates whatsapp queries when a conversation event arrives', async () => {
	const fake = fakeGraphClient()
	const { client } = await renderProbe(fake)
	const invalidate = vi.spyOn(client, 'invalidateQueries')

	expect(fake.documents[0]).toContain('whatsAppConversationEvent')

	fake.emit({ whatsAppConversationEvent: 'conversation-id' })

	expect(invalidate).toHaveBeenCalledWith({ queryKey: ['whatsapp'] })
})

test('ends the subscription on unmount', async () => {
	const fake = fakeGraphClient()
	const { view } = await renderProbe(fake)

	view.unmount()

	expect(fake.unsubscribes()).toBe(1)
})

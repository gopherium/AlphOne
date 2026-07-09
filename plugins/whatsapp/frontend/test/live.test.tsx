// SPDX-License-Identifier: AGPL-3.0-or-later

import { FakeEventSource } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { useLiveUpdates } from '../live'

function Probe() {
	useLiveUpdates()
	return null
}

function renderProbe() {
	const client = new QueryClient()
	const view = render(
		<QueryClientProvider client={client}>
			<Probe />
		</QueryClientProvider>,
	)
	return { client, view }
}

test('invalidates whatsapp queries when a live event arrives', () => {
	const { client } = renderProbe()
	const invalidate = vi.spyOn(client, 'invalidateQueries')
	const source = FakeEventSource.last()

	expect(source.url).toBe('/api/plugins/whatsapp/events')

	source.emit()

	expect(invalidate).toHaveBeenCalledWith({ queryKey: ['whatsapp'] })
})

test('closes the stream on unmount', () => {
	const { view } = renderProbe()
	const source = FakeEventSource.last()

	view.unmount()

	expect(source.closed).toBe(true)
})

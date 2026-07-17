// SPDX-License-Identifier: AGPL-3.0-or-later

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { useEventStream } from '../stream'
import { FakeEventSource } from '../testing'

function Probe() {
	useEventStream('/stream', { invalidateKeys: [['alpha'], ['beta']] })
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

const fetchMock = vi.fn()

beforeEach(() => {
	fetchMock.mockReset()
	vi.stubGlobal('fetch', fetchMock)
	vi.useFakeTimers()
	vi.spyOn(Math, 'random').mockReturnValue(1)
})

afterEach(() => {
	vi.useRealTimers()
	vi.restoreAllMocks()
})

test('refetches the given queries when an event arrives', () => {
	const { client } = renderProbe()
	const invalidate = vi.spyOn(client, 'invalidateQueries')
	const source = FakeEventSource.last()

	expect(source.url).toBe('/stream')

	source.emit()

	expect(invalidate).toHaveBeenCalledWith({ queryKey: ['alpha'] })
	expect(invalidate).toHaveBeenCalledWith({ queryKey: ['beta'] })
})

test('unmount closes the stream', () => {
	const { view } = renderProbe()

	view.unmount()

	expect(FakeEventSource.last().closed).toBe(true)
})

test('leaves browser-managed reconnects alone', () => {
	renderProbe()

	FakeEventSource.last().emitError(FakeEventSource.CONNECTING)

	expect(fetchMock).not.toHaveBeenCalled()
	expect(FakeEventSource.instances).toHaveLength(1)
})

test('hands a dead session to the login flow instead of reconnecting', async () => {
	const { client } = renderProbe()
	const invalidate = vi.spyOn(client, 'invalidateQueries')
	fetchMock.mockResolvedValue(new Response(null, { status: 401 }))

	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	await vi.runAllTimersAsync()

	expect(fetchMock).toHaveBeenCalledWith(
		'/api/auth/session',
		expect.objectContaining({ credentials: 'include' }),
	)
	expect(invalidate).toHaveBeenCalledWith({ queryKey: sessionQueryKey })
	expect(FakeEventSource.instances).toHaveLength(1)
})

test('recreates the stream after a transient failure with growing backoff', async () => {
	renderProbe()
	fetchMock.mockResolvedValue(new Response(null, { status: 200 }))

	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	await vi.advanceTimersByTimeAsync(0)
	await vi.advanceTimersByTimeAsync(999)
	expect(FakeEventSource.instances).toHaveLength(1)
	await vi.advanceTimersByTimeAsync(1)
	expect(FakeEventSource.instances).toHaveLength(2)

	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	await vi.advanceTimersByTimeAsync(0)
	await vi.advanceTimersByTimeAsync(1999)
	expect(FakeEventSource.instances).toHaveLength(2)
	await vi.advanceTimersByTimeAsync(1)
	expect(FakeEventSource.instances).toHaveLength(3)
})

test('refetches the given queries when a reconnect opens', async () => {
	const { client } = renderProbe()
	fetchMock.mockResolvedValue(new Response(null, { status: 200 }))

	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	await vi.advanceTimersByTimeAsync(0)
	await vi.advanceTimersByTimeAsync(1000)
	expect(FakeEventSource.instances).toHaveLength(2)

	const invalidate = vi.spyOn(client, 'invalidateQueries')
	FakeEventSource.last().emitOpen()

	expect(invalidate).toHaveBeenCalledWith({ queryKey: ['alpha'] })
	expect(invalidate).toHaveBeenCalledWith({ queryKey: ['beta'] })
})

test('a successful reconnect resets the backoff', async () => {
	renderProbe()
	fetchMock.mockResolvedValue(new Response(null, { status: 200 }))

	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	await vi.advanceTimersByTimeAsync(0)
	await vi.advanceTimersByTimeAsync(1000)
	expect(FakeEventSource.instances).toHaveLength(2)

	FakeEventSource.last().emitOpen()
	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	await vi.advanceTimersByTimeAsync(0)
	await vi.advanceTimersByTimeAsync(1000)
	expect(FakeEventSource.instances).toHaveLength(3)
})

test('keeps retrying when the probe itself fails', async () => {
	renderProbe()
	fetchMock.mockRejectedValue(new TypeError('network down'))

	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	await vi.advanceTimersByTimeAsync(0)
	await vi.advanceTimersByTimeAsync(1000)

	expect(FakeEventSource.instances).toHaveLength(2)
})

test('unmount cancels a pending reconnect', async () => {
	const { view } = renderProbe()
	fetchMock.mockResolvedValue(new Response(null, { status: 200 }))

	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	await vi.advanceTimersByTimeAsync(0)
	view.unmount()
	await vi.advanceTimersByTimeAsync(60_000)

	expect(FakeEventSource.instances).toHaveLength(1)
	expect(FakeEventSource.last().closed).toBe(true)
})

test('a probe answer arriving after unmount schedules nothing', async () => {
	const { view } = renderProbe()
	let resolveProbe: (response: Response) => void = () => {}
	fetchMock.mockImplementation(
		() =>
			new Promise((resolve) => {
				resolveProbe = resolve
			}),
	)

	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	view.unmount()
	resolveProbe(new Response(null, { status: 200 }))
	await vi.runAllTimersAsync()

	expect(FakeEventSource.instances).toHaveLength(1)
})

test('unmount aborts an in-flight probe', () => {
	const { view } = renderProbe()
	fetchMock.mockImplementation(() => new Promise(() => {}))

	FakeEventSource.last().emitError(FakeEventSource.CLOSED)
	view.unmount()

	const init = fetchMock.mock.calls[0]?.[1] as RequestInit
	expect((init.signal as AbortSignal).aborted).toBe(true)
})

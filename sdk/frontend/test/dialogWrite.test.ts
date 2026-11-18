// SPDX-License-Identifier: AGPL-3.0-or-later

import { gql } from 'urql'
import { expect, onTestFinished, test, vi } from 'vitest'

import { dialogWrite } from '../dialogWrite'
import { createGraphClient } from '../graph'
import { HttpResponse, graphql, server } from '../testing'

const renameMutation = gql`
	mutation RenameContact($id: UUID!, $name: String!) {
		renameContact(id: $id, name: $name) {
			id
			name
		}
	}
`

/** renamed is the answer of a rename that worked. */
const renamed = { renameContact: { __typename: 'Contact', id: 'c1', name: 'Maria Perez' } }

/** The variables every write in these tests sends. */
const sent = { id: 'c1', name: 'Maria Perez' }

/**
 * Returns the graph client the writes go through.
 * @returns The urql client.
 */
function newClient() {
	return createGraphClient({ onSessionExpired: vi.fn() }).client
}

/**
 * Records the signal of every request the client sends, the requests still reaching the test server.
 * @returns The signals, in the order the requests went.
 */
function recordSignals() {
	const signals: AbortSignal[] = []
	const send = globalThis.fetch
	const spied = vi.spyOn(globalThis, 'fetch').mockImplementation((input, init) => {
		signals.push(init?.signal as AbortSignal)
		return send(input, init)
	})
	onTestFinished(() => spied.mockRestore())
	return signals
}

/**
 * Holds every rename at the test server without an answer.
 * @returns A promise that settles once a rename reached the server.
 */
function holdRenames() {
	let arrive = () => {}
	const arrived = new Promise<void>((resolve) => {
		arrive = resolve
	})
	server.use(
		graphql.mutation('RenameContact', async () => {
			arrive()
			await new Promise(() => {})
		}),
	)
	return arrived
}

/** Takes AbortSignal.any away until the test ends, as in a browser that lacks it. */
function withoutSignalAny() {
	const any = Object.getOwnPropertyDescriptor(AbortSignal, 'any') as PropertyDescriptor
	Object.defineProperty(AbortSignal, 'any', { value: undefined, configurable: true, writable: true })
	onTestFinished(() => {
		Object.defineProperty(AbortSignal, 'any', any)
	})
}

test('answers what the write answered while the signal never fires', async () => {
	server.use(graphql.mutation('RenameContact', () => HttpResponse.json({ data: renamed })))

	const answer = await dialogWrite(newClient(), renameMutation, sent, new AbortController().signal)

	expect(answer.data).toMatchObject({ renameContact: { id: 'c1', name: 'Maria Perez' } })
	expect(answer.error).toBeUndefined()
})

test('answers a rejected write with its error, for the dialog to put into words', async () => {
	server.use(
		graphql.mutation('RenameContact', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'name rejected', extensions: { code: 'VALIDATION' } }] }),
		),
	)

	const answer = await dialogWrite(newClient(), renameMutation, sent, new AbortController().signal)

	expect(answer.error?.graphQLErrors[0].message).toBe('name rejected')
})

test('stops the request and rejects with the reason of the signal once it fires', async () => {
	const arrived = holdRenames()
	const signals = recordSignals()
	const timing = new AbortController()
	const reason = new DOMException('The write ran out of time.', 'TimeoutError')

	const answer = dialogWrite(newClient(), renameMutation, sent, timing.signal)
	await arrived
	timing.abort(reason)

	await expect(answer).rejects.toBe(reason)
	expect(signals).toHaveLength(1)
	await vi.waitFor(() => expect(signals[0].aborted).toBe(true))
})

test('sends nothing and rejects at once when the signal fired before the write', async () => {
	const reached = vi.fn()
	server.use(
		graphql.mutation('RenameContact', () => {
			reached()
			return HttpResponse.json({ data: renamed })
		}),
	)
	const timing = new AbortController()
	timing.abort('out of time')

	await expect(dialogWrite(newClient(), renameMutation, sent, timing.signal)).rejects.toBe('out of time')
	await new Promise((resolve) => setTimeout(resolve, 30))
	expect(reached).not.toHaveBeenCalled()
})

test('sends nothing once the signal fires before the request leaves', async () => {
	const reached = vi.fn()
	server.use(
		graphql.mutation('RenameContact', () => {
			reached()
			return HttpResponse.json({ data: renamed })
		}),
	)
	const signals = recordSignals()
	const timing = new AbortController()

	const answer = dialogWrite(newClient(), renameMutation, sent, timing.signal)
	timing.abort('out of time')

	await expect(answer).rejects.toBe('out of time')
	await vi.waitFor(() => expect(signals).toHaveLength(1))
	expect(signals[0].aborted).toBe(true)
	expect(reached).not.toHaveBeenCalled()
})

test('leaves nothing listening on the signal once the answer came', async () => {
	server.use(graphql.mutation('RenameContact', () => HttpResponse.json({ data: renamed })))
	const timing = new AbortController()
	const added = vi.spyOn(timing.signal, 'addEventListener')
	const removed = vi.spyOn(timing.signal, 'removeEventListener')

	await dialogWrite(newClient(), renameMutation, sent, timing.signal)

	expect(added).toHaveBeenCalled()
	expect(removed.mock.calls.map(([type, listener]) => [type, listener])).toEqual(
		added.mock.calls.map(([type, listener]) => [type, listener]),
	)
})

test('stops the write in a browser without AbortSignal.any', async () => {
	withoutSignalAny()
	const arrived = holdRenames()
	const signals = recordSignals()
	const timing = new AbortController()
	const reason = new DOMException('The write ran out of time.', 'TimeoutError')

	const answer = dialogWrite(newClient(), renameMutation, sent, timing.signal)
	await arrived
	timing.abort(reason)

	await expect(answer).rejects.toBe(reason)
	await vi.waitFor(() => expect(signals[0].aborted).toBe(true))
})

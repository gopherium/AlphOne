// SPDX-License-Identifier: AGPL-3.0-or-later

import { Client, fetchExchange } from 'urql'
import { expect, test, vi } from 'vitest'

import { graphRetryExchange } from '../graph'

const versionQuery = 'query Version { version }'
const addTaskMutation = 'mutation AddTask { createTask }'

/** Returns a client over a fetch answering with the queued statuses in order. */
function clientRefusing(statuses: number[]) {
	const fetchStub = vi.fn(async () => {
		const status = statuses.shift() ?? 200
		if (status === 200) {
			return new Response(JSON.stringify({ data: { version: '9.9.9' } }), {
				headers: { 'content-type': 'application/json' },
			})
		}
		return new Response(JSON.stringify({ error: 'too many concurrent operations' }), {
			status,
			headers: { 'content-type': 'application/json' },
		})
	})
	const client = new Client({
		url: '/api/graphql',
		exchanges: [graphRetryExchange(), fetchExchange],
		fetch: fetchStub as unknown as typeof fetch,
	})
	return { client, fetchStub }
}

test('resends a query the server refused for concurrency', async () => {
	const { client, fetchStub } = clientRefusing([429])

	const result = await client.query(versionQuery, {}).toPromise()

	expect(result.data).toEqual({ version: '9.9.9' })
	expect(fetchStub).toHaveBeenCalledTimes(2)
})

test('gives up on a query the server keeps refusing', async () => {
	const { client, fetchStub } = clientRefusing([429, 429, 429, 429, 429])

	const result = await client.query(versionQuery, {}).toPromise()

	expect(result.error).toBeDefined()
	expect(fetchStub).toHaveBeenCalledTimes(3)
})

test('leaves a refused mutation to the caller, so no write is sent twice', async () => {
	const { client, fetchStub } = clientRefusing([429, 429, 429])

	const result = await client.mutation(addTaskMutation, {}).toPromise()

	expect(result.error).toBeDefined()
	expect(fetchStub).toHaveBeenCalledTimes(1)
})

test('leaves a failure the server did not blame on concurrency', async () => {
	const { client, fetchStub } = clientRefusing([500, 500])

	const result = await client.query(versionQuery, {}).toPromise()

	expect(result.error).toBeDefined()
	expect(fetchStub).toHaveBeenCalledTimes(1)
})

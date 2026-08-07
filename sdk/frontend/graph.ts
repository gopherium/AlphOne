// SPDX-License-Identifier: AGPL-3.0-or-later

import { UnauthorizedError } from '@gopherium/react-auth'
import { cacheExchange } from '@urql/exchange-graphcache'
import { relayPagination } from '@urql/exchange-graphcache/extras'
import { Client, fetchExchange, getOperationName } from 'urql'
import type { CombinedError, Exchange, Operation } from 'urql'
import { pipe, tap } from 'wonka'

import { ValidationError } from './errors'

/** The graph endpoint every operation is posted to. */
const graphEndpoint = '/api/graphql'

/**
 * Returns the extensions code the first graph error of a failure carries.
 * @param error - The failure a graph operation answered with.
 * @returns The code, or undefined for a transport failure.
 */
function errorCode(error: CombinedError): string | undefined {
	const code = error.graphQLErrors[0]?.extensions?.code
	return typeof code === 'string' ? code : undefined
}

/**
 * Maps a graph failure onto the error class the screens branch on.
 * @param error - The failure a graph operation answered with.
 * @returns The mapped error, or undefined when the operation succeeded.
 */
export function graphError(error: CombinedError | undefined): Error | undefined {
	if (!error) {
		return undefined
	}
	const message = error.graphQLErrors[0]?.message ?? error.message
	switch (errorCode(error)) {
		case 'UNAUTHENTICATED':
			return new UnauthorizedError(message)
		case 'VALIDATION':
			return new ValidationError(message)
		default:
			return new Error(message)
	}
}

/**
 * Returns an exchange announcing every operation the graph rejects as unauthenticated.
 * @param onSessionExpired - Called once per rejected operation.
 * @returns The exchange reporting expiry as a side effect.
 */
function sessionExchange(onSessionExpired: () => void): Exchange {
	return ({ forward }) =>
		(operations$) =>
			pipe(
				forward(operations$),
				tap((result) => {
					if (result.error && errorCode(result.error) === 'UNAUTHENTICATED') {
						onSessionExpired()
					}
				}),
			)
}

/** GraphClient carries the configured client beside its doorbell. */
export interface GraphClient {
	/** client serves every graph operation the screens run. */
	client: Client
	/** refetch reruns the named active queries against the network. */
	refetch: (operations: readonly string[]) => void
}

/**
 * Returns the normalized cache exchange configured for the AlphOne schema.
 * @returns The cache exchange.
 */
function graphCacheExchange(): Exchange {
	return cacheExchange({
		keys: {
			CreateTaskPayload: () => null,
			CreateWebhookPayload: () => null,
			ImportCommitPayload: () => null,
			LoginPayload: () => null,
			ImportAssignment: () => null,
			WhatsAppMedia: () => null,
			ImportContact: (data) => data.rowId as string,
			ImportField: (data) => data.name as string,
		},
		resolvers: {
			Query: {
				contacts: relayPagination(),
				tasks: relayPagination(),
			},
			Contact: {
				tasks: relayPagination(),
			},
		},
	})
}

/**
 * Returns an exchange tracking active queries beside the trigger rerunning them.
 * @returns The exchange and the trigger naming which queries to rerun.
 */
function doorbellExchange(): { exchange: Exchange; refetch: GraphClient['refetch'] } {
	const active = new Map<number, Operation>()
	let graphClient!: Client
	const exchange: Exchange = ({ client, forward }) => {
		graphClient = client
		return (operations$) =>
			forward(
				pipe(
					operations$,
					tap((operation) => {
						if (operation.kind === 'query') {
							active.set(operation.key, operation)
						} else if (operation.kind === 'teardown') {
							active.delete(operation.key)
						}
					}),
				),
			)
	}
	const refetch: GraphClient['refetch'] = (operations) => {
		const wanted = new Set<string | undefined>(operations)
		for (const operation of active.values()) {
			if (wanted.has(getOperationName(operation.query))) {
				graphClient.reexecuteOperation(
					graphClient.createRequestOperation('query', operation, {
						...operation.context,
						requestPolicy: 'network-only',
					}),
				)
			}
		}
	}
	return { exchange, refetch }
}

/**
 * Returns the configured graph client every screen and plugin shares.
 * @param options - The session expiry callback bridging to the auth layer.
 * @returns The client beside its doorbell trigger.
 */
export function createGraphClient(options: { onSessionExpired: () => void }): GraphClient {
	const doorbell = doorbellExchange()
	const client = new Client({
		url: graphEndpoint,
		fetchOptions: { credentials: 'same-origin' },
		preferGetMethod: false,
		exchanges: [
			doorbell.exchange,
			graphCacheExchange(),
			sessionExchange(options.onSessionExpired),
			fetchExchange,
		],
	})
	return { client, refetch: doorbell.refetch }
}

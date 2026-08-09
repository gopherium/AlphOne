// SPDX-License-Identifier: AGPL-3.0-or-later

import { UnauthorizedError } from '@gopherium/react-auth'
import { cacheExchange } from '@urql/exchange-graphcache'
import { relayPagination } from '@urql/exchange-graphcache/extras'
import { retryExchange } from '@urql/exchange-retry'
import { createClient as createEventStreamClient } from 'graphql-sse'
import { Client, fetchExchange, getOperationName, subscriptionExchange } from 'urql'
import type { CombinedError, Exchange, Operation } from 'urql'
import { pipe, tap } from 'wonka'

import { ValidationError } from './errors'

/** The graph endpoint every operation is posted to. */
const graphEndpoint = '/api/graphql'

/** The bounds of the wait between event stream reconnections. */
const initialRetryDelay = 1_000
const maxRetryDelay = 30_000

/**
 * Returns how long to wait before the next event stream reconnection.
 * @param retries - How many reconnections have already been attempted.
 * @returns The delay in milliseconds, capped and jittered.
 */
function retryDelay(retries: number): number {
	const delay = Math.min(initialRetryDelay * 2 ** retries, maxRetryDelay)
	return delay / 2 + (Math.random() * delay) / 2
}

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
 * Returns the extensions the first graph error of a failure carries.
 * @param error - The failure a graph operation answered with.
 * @returns The extensions, empty when the operation succeeded.
 */
export function graphExtensions(error: CombinedError | undefined): Record<string, unknown> {
	return (error?.graphQLErrors[0]?.extensions ?? {}) as Record<string, unknown>
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
		case 'CONFLICT':
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
	/** onStreamOpen calls the listener on every event stream connection, returning its removal. */
	onStreamOpen: (listener: () => void) => () => void
}

/**
 * Returns the subscription exchange beside the registrar for its connections.
 * @returns The exchange and the registrar taking connection listeners.
 */
function eventStreamExchange(): { exchange: Exchange; onStreamOpen: GraphClient['onStreamOpen'] } {
	const listeners = new Set<() => void>()
	const stream = createEventStreamClient({
		url: graphEndpoint,
		credentials: 'same-origin',
		retryAttempts: Infinity,
		retry: (retries) =>
			new Promise((resolve) => {
				setTimeout(resolve, retryDelay(retries))
			}),
		on: {
			connected: () => {
				for (const listener of listeners) {
					listener()
				}
			},
		},
	})
	const exchange = subscriptionExchange({
		forwardSubscription: (request) => ({
			subscribe: (sink) => ({
				unsubscribe: stream.subscribe({ ...request, query: request.query as string }, sink),
			}),
		}),
	})
	return {
		exchange,
		onStreamOpen: (listener) => {
			listeners.add(listener)
			return () => {
				listeners.delete(listener)
			}
		},
	}
}

/** The bounds of the wait between resends of a refused read. */
const initialResendDelay = 100
const maxResendDelay = 1_000
const maxResendAttempts = 3

/**
 * Returns whether a failure is the graph endpoint refusing a read for concurrency.
 * @param error - The failure a graph operation answered with.
 * @param operation - The operation that failed.
 * @returns Whether the operation is worth resending.
 */
function refusedRead(error: CombinedError, operation: Operation): boolean {
	return operation.kind === 'query' && (error.response as Response | undefined)?.status === 429
}

/**
 * Returns the exchange resending a read the graph endpoint refused for concurrency.
 * @returns The retry exchange.
 */
export function graphRetryExchange(): Exchange {
	return retryExchange({
		initialDelayMs: initialResendDelay,
		maxDelayMs: maxResendDelay,
		maxNumberAttempts: maxResendAttempts,
		randomDelay: true,
		retryIf: refusedRead,
	})
}

/**
 * Returns the normalized cache exchange configured for the AlphOne schema.
 * @returns The cache exchange.
 */
export function graphCacheExchange(): Exchange {
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
export function doorbellExchange(): { exchange: Exchange; refetch: GraphClient['refetch'] } {
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
	const stream = eventStreamExchange()
	const client = new Client({
		url: graphEndpoint,
		fetchOptions: { credentials: 'same-origin' },
		preferGetMethod: false,
		exchanges: [
			doorbell.exchange,
			graphCacheExchange(),
			sessionExchange(options.onSessionExpired),
			stream.exchange,
			graphRetryExchange(),
			fetchExchange,
		],
	})
	return { client, refetch: doorbell.refetch, onStreamOpen: stream.onStreamOpen }
}

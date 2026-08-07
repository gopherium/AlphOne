// SPDX-License-Identifier: AGPL-3.0-or-later

import { isSessionRevoked, sessionQueryKey } from '@gopherium/react-auth'
import { useQueryClient } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'

import type { GraphClient } from './graph'

const initialRetryDelay = 1_000
const maxRetryDelay = 30_000

/**
 * useServerEvents subscribes to a host SSE endpoint, announcing every event and
 * restoring the subscription after failures.
 * @param url - The SSE endpoint to subscribe to.
 * @param onEvent - Called once per event and once per established connection.
 */
function useServerEvents(url: string, onEvent: () => void) {
	const queryClient = useQueryClient()
	const announce = useRef(onEvent)
	announce.current = onEvent
	useEffect(() => {
		let source: EventSource
		let retryTimer: ReturnType<typeof setTimeout> | undefined
		let probe: AbortController | undefined
		let attempts = 0
		let disposed = false

		/**
		 * Arms the next connection attempt with capped exponential backoff
		 * and jitter.
		 */
		function scheduleReconnect() {
			if (disposed) {
				return
			}
			const delay = Math.min(initialRetryDelay * 2 ** attempts, maxRetryDelay)
			attempts += 1
			retryTimer = setTimeout(connect, delay / 2 + (Math.random() * delay) / 2)
		}

		/**
		 * Asks the session endpoint whether a permanent stream failure means
		 * the session is gone or the outage is transient.
		 */
		function probeSession() {
			probe = new AbortController()
			isSessionRevoked(probe.signal)
				.then((revoked) => {
					if (revoked) {
						void queryClient.invalidateQueries({ queryKey: sessionQueryKey })
						return
					}
					scheduleReconnect()
				})
				.catch(scheduleReconnect)
		}

		/**
		 * Announces one event to the subscriber.
		 */
		function announceEvent() {
			announce.current()
		}

		/**
		 * Opens the event stream and installs its lifecycle handlers.
		 */
		function connect() {
			source = new EventSource(url)
			source.onopen = () => {
				attempts = 0
				announceEvent()
			}
			source.onmessage = announceEvent
			source.onerror = () => {
				if (source.readyState !== EventSource.CLOSED) {
					return
				}
				probeSession()
			}
		}

		connect()
		return () => {
			disposed = true
			source.close()
			if (retryTimer !== undefined) {
				clearTimeout(retryTimer)
			}
			probe?.abort()
		}
	}, [url, queryClient])
}

/**
 * useEventStream subscribes to a host SSE endpoint, refetching the given
 * queries on every event and restoring the subscription after failures.
 * @param url - The SSE endpoint to subscribe to.
 * @param options - The query keys to refetch when the stream reports a change.
 */
export function useEventStream(url: string, options: { invalidateKeys: readonly QueryKey[] }) {
	const queryClient = useQueryClient()
	const serializedKeys = JSON.stringify(options.invalidateKeys)
	useServerEvents(url, () => {
		for (const queryKey of JSON.parse(serializedKeys) as QueryKey[]) {
			void queryClient.invalidateQueries({ queryKey })
		}
	})
}

/**
 * useGraphStream subscribes to a host SSE endpoint, rerunning the named graph
 * operations on every event and restoring the subscription after failures.
 * @param url - The SSE endpoint to subscribe to.
 * @param options - The graph client and the operation names to rerun.
 */
export function useGraphStream(
	url: string,
	options: { graph: GraphClient; operations: readonly string[] },
) {
	const { graph } = options
	const serializedOperations = JSON.stringify(options.operations)
	useServerEvents(url, () => {
		graph.refetch(JSON.parse(serializedOperations) as string[])
	})
}

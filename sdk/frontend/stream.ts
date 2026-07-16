// SPDX-License-Identifier: AGPL-3.0-or-later

import { useQueryClient } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'
import { useEffect } from 'react'

/**
 * sessionQueryKey is the react-query key the host app stores the login
 * session under.
 */
export const sessionQueryKey = ['session'] as const

const initialRetryDelay = 1_000
const maxRetryDelay = 30_000

/**
 * useEventStream subscribes to a host SSE endpoint, refetching the given
 * queries on every event and restoring the subscription after failures.
 * @param url - The SSE endpoint to subscribe to.
 * @param options - The query keys to refetch when the stream reports a change.
 */
export function useEventStream(url: string, options: { invalidateKeys: readonly QueryKey[] }) {
	const queryClient = useQueryClient()
	const serializedKeys = JSON.stringify(options.invalidateKeys)
	useEffect(() => {
		const invalidateKeys = JSON.parse(serializedKeys) as QueryKey[]
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
			fetch('/api/auth/session', { credentials: 'include', signal: probe.signal })
				.then((response) => {
					if (response.status === 401) {
						void queryClient.invalidateQueries({ queryKey: sessionQueryKey })
						return
					}
					scheduleReconnect()
				})
				.catch(scheduleReconnect)
		}

		/**
		 * Refetches every query the stream reports on.
		 */
		function invalidateAll() {
			for (const queryKey of invalidateKeys) {
				void queryClient.invalidateQueries({ queryKey })
			}
		}

		/**
		 * Opens the event stream and installs its lifecycle handlers.
		 */
		function connect() {
			source = new EventSource(url)
			source.onopen = () => {
				attempts = 0
				invalidateAll()
			}
			source.onmessage = invalidateAll
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
	}, [url, queryClient, serializedKeys])
}

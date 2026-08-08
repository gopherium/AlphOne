// SPDX-License-Identifier: AGPL-3.0-or-later

import { useQueryClient } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { useSubscription } from 'urql'

import type { GraphClient } from './graph'

/** The subscription announcing every core event the caller may see. */
const coreEventSubscription = `subscription CoreEvent { coreEvent }`

/**
 * useGraphEvents announces every frame the subscription delivers and every
 * connection the event stream establishes.
 * @param graph - The graph client serving the subscription.
 * @param query - The subscription document to run.
 * @param onEvent - Called once per frame and once per established connection.
 */
export function useGraphEvents(graph: GraphClient, query: string, onEvent: () => void) {
	const announce = useRef(onEvent)
	announce.current = onEvent
	useSubscription({ query }, () => {
		announce.current()
		return undefined
	})
	useEffect(() => graph.onStreamOpen(() => announce.current()), [graph])
}

/**
 * useGraphStream reruns the named graph operations on every core event,
 * catching up whenever the event stream reconnects.
 * @param options - The graph client, the operation names to rerun, and the query keys to refetch.
 */
export function useGraphStream(options: {
	graph: GraphClient
	operations: readonly string[]
	invalidateKeys?: readonly QueryKey[]
}) {
	const { graph } = options
	const queryClient = useQueryClient()
	const serializedOperations = JSON.stringify(options.operations)
	const serializedKeys = JSON.stringify(options.invalidateKeys ?? [])
	useGraphEvents(graph, coreEventSubscription, () => {
		graph.refetch(JSON.parse(serializedOperations) as string[])
		for (const queryKey of JSON.parse(serializedKeys) as QueryKey[]) {
			void queryClient.invalidateQueries({ queryKey })
		}
	})
}

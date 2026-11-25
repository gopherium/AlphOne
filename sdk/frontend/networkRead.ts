// SPDX-License-Identifier: AGPL-3.0-or-later

import type { Client, OperationResult, TypedDocumentNode } from 'urql'
import { filter, pipe, subscribe, take } from 'wonka'

/**
 * Reads one query from the network, dropping the read once the signal fires.
 * @param client - The graph client reading it.
 * @param query - The query to read, which takes no variables.
 * @param signal - The signal that drops the read.
 * @returns The first fresh answer the graph sent.
 */
export function readFromNetwork<Data>(
	client: Client,
	query: TypedDocumentNode<Data, Record<string, never>>,
	signal: AbortSignal,
): Promise<OperationResult<Data, Record<string, never>>> {
	return new Promise((resolve) => {
		const reading = pipe(
			client.query(query, {}, { requestPolicy: 'network-only' }),
			filter((answer) => !answer.stale),
			take(1),
			subscribe(resolve),
		)
		signal.addEventListener('abort', reading.unsubscribe, { once: true })
	})
}

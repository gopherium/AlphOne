// SPDX-License-Identifier: AGPL-3.0-or-later

import type { AnyVariables, Client, DocumentInput, OperationResult } from 'urql'
import { pipe, subscribe, take } from 'wonka'

import { graphFetch } from './graph'

/**
 * Returns a signal that fires with the reason of whichever given signal fires first.
 * @param signals - The signals it follows.
 * @returns The signal following them.
 */
function firstOf(...signals: AbortSignal[]): AbortSignal {
	const first = new AbortController()
	for (const signal of signals) {
		if (signal.aborted) {
			first.abort(signal.reason)
			return first.signal
		}
		signal.addEventListener('abort', () => first.abort(signal.reason), { once: true })
	}
	return first.signal
}

/**
 * Returns the graph fetch with every request it sends also stopped once the given signal fires.
 * @param signal - The signal that stops the write.
 * @returns The fetch the write goes through.
 */
function stoppedBy(signal: AbortSignal): typeof graphFetch {
	return (input, init) =>
		graphFetch(input, { ...init, signal: firstOf((init as RequestInit).signal as AbortSignal, signal) })
}

/**
 * Sends a write a dialog waits on, stopping its request and rejecting with the signal's reason once the signal fires.
 * @param client - The graph client the write goes through.
 * @param query - The mutation.
 * @param variables - The variables of the mutation.
 * @param signal - The signal the dialog hands its write.
 * @returns The answer of the write, its error included, or a rejection once the signal fired.
 */
export function dialogWrite<Data = unknown, Variables extends AnyVariables = AnyVariables>(
	client: Client,
	query: DocumentInput<Data, Variables>,
	variables: Variables,
	signal: AbortSignal,
): Promise<OperationResult<Data, Variables>> {
	return new Promise((resolve, reject) => {
		if (signal.aborted) {
			reject(signal.reason)
			return
		}
		const stopping = new AbortController()
		const stop = () => {
			stopping.abort(signal.reason)
			sending.unsubscribe()
			reject(signal.reason)
		}
		const sending = pipe(
			client.mutation(query, variables, { fetch: stoppedBy(stopping.signal) }),
			take(1),
			subscribe((answer: OperationResult<Data, Variables>) => {
				signal.removeEventListener('abort', stop)
				resolve(answer)
			}),
		)
		signal.addEventListener('abort', stop, { once: true })
	})
}

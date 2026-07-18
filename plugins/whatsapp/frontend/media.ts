import { useQuery } from '@tanstack/react-query'

import { mediaURL } from './api'

const maxConcurrentMediaFetches = 2

let activeFetches = 0
const waiters: Array<() => void> = []

/**
 * Acquires a media download slot, waiting while both slots are busy.
 * @returns A promise resolving once a slot is held.
 */
function acquire(): Promise<void> {
	return new Promise((resolve) => {
		if (activeFetches < maxConcurrentMediaFetches) {
			activeFetches += 1
			resolve()
			return
		}
		waiters.push(() => {
			activeFetches += 1
			resolve()
		})
	})
}

/**
 * Releases a media download slot, waking the next waiter if any.
 */
function release(): void {
	activeFetches -= 1
	const next = waiters.shift()
	if (next) {
		next()
	}
}

/**
 * Downloads a media blob and exposes it as an object URL, keeping at most
 * two downloads in flight across the app.
 * @param conversationId - The conversation owning the message.
 * @param messageId - The message whose blob to download.
 * @returns A promise resolving to the blob's object URL.
 */
async function fetchMediaObjectURL(
	conversationId: string,
	messageId: string,
): Promise<string> {
	await acquire()
	try {
		const response = await fetch(mediaURL(conversationId, messageId))
		if (!response.ok) {
			throw new Error(`fetching media failed with status ${response.status}`)
		}
		return URL.createObjectURL(await response.blob())
	} finally {
		release()
	}
}

/**
 * useMediaBlob loads a message's stored blob once and caches its object URL
 * for the session, outside the live-invalidated whatsapp namespace.
 * @param conversationId - The conversation owning the message.
 * @param messageId - The message whose blob to load.
 * @returns The object URL query.
 */
export function useMediaBlob(conversationId: string, messageId: string) {
	return useQuery({
		queryKey: ['whatsapp-media', messageId],
		queryFn: () => fetchMediaObjectURL(conversationId, messageId),
		staleTime: Infinity,
		gcTime: Infinity,
	})
}

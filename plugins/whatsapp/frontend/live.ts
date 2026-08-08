// SPDX-License-Identifier: AGPL-3.0-or-later

import { useGraph, useGraphEvents } from '@alphone/frontend-sdk'

/** The subscription announcing every conversation the plugin changes. */
const conversationEventSubscription = `subscription WhatsAppConversationEvent { whatsAppConversationEvent }`

/**
 * useLiveUpdates refetches the plugin's queries whenever the server reports a
 * conversation change.
 */
export function useLiveUpdates() {
	const graph = useGraph()
	useGraphEvents(graph, conversationEventSubscription, () => {
		graph.refetch(['WhatsAppConversations', 'WhatsAppThread'])
	})
}

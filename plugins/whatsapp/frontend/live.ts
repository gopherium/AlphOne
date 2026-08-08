// SPDX-License-Identifier: AGPL-3.0-or-later

import { useGraph, useGraphEvents } from '@alphone/frontend-sdk'
import { useQueryClient } from '@tanstack/react-query'

/** The subscription announcing every conversation the plugin changes. */
const conversationEventSubscription = `subscription WhatsAppConversationEvent { whatsAppConversationEvent }`

/**
 * useLiveUpdates refetches the plugin's queries whenever the server reports a
 * conversation change.
 */
export function useLiveUpdates() {
	const graph = useGraph()
	const queryClient = useQueryClient()
	useGraphEvents(graph, conversationEventSubscription, () => {
		graph.refetch(['WhatsAppConversations'])
		void queryClient.invalidateQueries({ queryKey: ['whatsapp'] })
	})
}

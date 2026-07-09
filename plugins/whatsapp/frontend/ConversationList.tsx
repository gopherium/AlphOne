// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Text } from '@alphone/frontend-sdk'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import { fetchConversations } from './api'
import { useLiveUpdates } from './live'

/**
 * Renders the WhatsApp conversation list for the sidebar, with live updates.
 * @returns The conversation links, or a status message while loading, on error,
 * or when empty.
 */
export function ConversationList() {
	useLiveUpdates()
	const conversations = useQuery({
		queryKey: ['whatsapp', 'conversations'],
		queryFn: fetchConversations,
	})

	if (conversations.isPending) {
		return <Text>Loading conversations…</Text>
	}
	if (conversations.isError) {
		return <Text>Conversations could not be loaded.</Text>
	}
	if (conversations.data.length === 0) {
		return <Text>No conversations yet.</Text>
	}
	return (
		<ul className="alphone-conversations">
			{conversations.data.map((conversation) => (
				<li key={conversation.id}>
					<Link
						to="/whatsapp/conversations/$conversationId"
						params={{ conversationId: conversation.id }}
						className="alphone-conversation"
					>
						<span className="alphone-conversation__name">
							{conversation.contact_name}
						</span>
						<Badge>{conversation.status}</Badge>
					</Link>
				</li>
			))}
		</ul>
	)
}

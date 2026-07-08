// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Text } from '@alphone/frontend-sdk'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import { fetchConversations } from './api'
import { useLiveUpdates } from './live'

/**
 * Renders the WhatsApp conversation inbox list with live updates.
 * @returns A list of conversation links with status badges, or a status message while loading, on error, or when empty.
 */
export function Inbox() {
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
		<ul>
			{conversations.data.map((conversation) => (
				<li key={conversation.id}>
					<Link
						to="/whatsapp/conversations/$conversationId"
						params={{ conversationId: conversation.id }}
					>
						{conversation.contact_name}
					</Link>
					<Badge>{conversation.status}</Badge>
				</li>
			))}
		</ul>
	)
}

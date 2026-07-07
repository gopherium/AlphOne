// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Text } from '@alphone/frontend-sdk'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import { fetchConversations } from './api'

export function Inbox() {
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

// SPDX-License-Identifier: AGPL-3.0-or-later

import { useQuery } from '@tanstack/react-query'

import { fetchConversations } from '../api/whatsapp'
import { Badge, Text } from '../ui'

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
					<Text>{conversation.contact_name}</Text>
					<Badge>{conversation.status}</Badge>
				</li>
			))}
		</ul>
	)
}

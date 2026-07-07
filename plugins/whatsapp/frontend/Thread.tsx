// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Text } from '@alphone/frontend-sdk'
import { useQuery } from '@tanstack/react-query'

import { fetchMessages } from './api'

export function Thread({ conversationId }: { conversationId: string }) {
	const messages = useQuery({
		queryKey: ['whatsapp', 'messages', conversationId],
		queryFn: () => fetchMessages(conversationId),
	})

	if (messages.isPending) {
		return <Text>Loading messages…</Text>
	}
	if (messages.isError) {
		return <Text>Messages could not be loaded.</Text>
	}
	if (messages.data.length === 0) {
		return <Text>No messages yet.</Text>
	}
	return (
		<ul>
			{messages.data.map((message) => (
				<li key={message.id}>
					<Text>{message.content}</Text>
					<Badge>{message.direction}</Badge>
				</li>
			))}
		</ul>
	)
}

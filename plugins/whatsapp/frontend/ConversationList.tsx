// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Text, VisuallyHidden } from '@alphone/frontend-sdk'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import { fetchConversations } from './api'
import { formatListTime } from './format'
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
		return <Text role="status">Loading conversations…</Text>
	}
	if (conversations.isError) {
		return <Text role="alert">Conversations could not be loaded.</Text>
	}
	if (conversations.data.length === 0) {
		return <Text role="status">No conversations yet.</Text>
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
						<span className="alphone-conversation__top">
							<span className="alphone-conversation__name">
								{conversation.contact_name}
							</span>
							<time
								className="alphone-conversation__time"
								dateTime={conversation.last_activity_at.toISOString()}
							>
								{formatListTime(conversation.last_activity_at, new Date())}
							</time>
						</span>
						<span className="alphone-conversation__bottom">
							<span className="alphone-conversation__preview">
								{conversation.last_message_preview ?? ''}
							</span>
							<VisuallyHidden render={<span />}>status</VisuallyHidden>
							<Badge>{conversation.status}</Badge>
						</span>
					</Link>
				</li>
			))}
		</ul>
	)
}

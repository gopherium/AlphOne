// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
	LoadingRows,
	NavigationItem,
	NavigationList,
	Text,
	VisuallyHidden,
	__,
	comment,
	useGraphQuery,
} from '@alphone/frontend-sdk'

import { formatListTime } from './format'
import { useLiveUpdates } from './live'
import { conversationsQuery } from './operations'

/**
 * Renders the WhatsApp conversation list for the sidebar, with live updates.
 * @returns The conversation rows, or a status message while loading, on error,
 * or when empty.
 */
export function ConversationList() {
	useLiveUpdates()
	const [conversations] = useGraphQuery({
		query: conversationsQuery,
		requestPolicy: 'cache-and-network',
	})
	const rows = conversations.data?.whatsAppConversations

	if (conversations.error) {
		return <Text role="alert">{__('Conversations could not be loaded.', 'alphone-whatsapp')}</Text>
	}
	if (!rows) {
		return <LoadingRows label={__('Loading conversations…', 'alphone-whatsapp')} />
	}
	if (rows.length === 0) {
		return <Text role="status">{__('No conversations yet.', 'alphone-whatsapp')}</Text>
	}
	return (
		<NavigationList>
			{rows.map((conversation) => (
				<NavigationItem
					key={conversation.id}
					to="/whatsapp/conversations/$conversationId"
					params={{ conversationId: conversation.id }}
					icon={comment}
				>
					<span className="alphone-conversation">
						<span className="alphone-conversation__top">
							<span className="alphone-conversation__name">
								{conversation.contact.name}
							</span>
							<time
								className="alphone-conversation__time"
								dateTime={conversation.lastActivityAt}
							>
								{formatListTime(new Date(conversation.lastActivityAt), new Date())}
							</time>
						</span>
						<span className="alphone-conversation__bottom">
							<span className="alphone-conversation__preview">
								{conversation.lastMessagePreview ?? ''}
							</span>
							<VisuallyHidden render={<span />}>
								{__('status', 'alphone-whatsapp')}
							</VisuallyHidden>
							<Badge>{conversation.status}</Badge>
						</span>
					</span>
				</NavigationItem>
			))}
		</NavigationList>
	)
}

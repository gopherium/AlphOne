// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from './gql'

export const conversationsQuery = graphql(`
	query WhatsAppConversations {
		whatsAppConversations {
			id
			status
			lastActivityAt
			lastMessagePreview
			contact {
				id
				name
			}
		}
	}
`)

export const threadQuery = graphql(`
	query WhatsAppThread($conversationId: UUID!) {
		whatsAppConversation(id: $conversationId) {
			id
			contact {
				id
				name
			}
			messages {
				...ThreadMessage
			}
		}
	}
`)

/** @public The message fields every thread document selects. */
export const threadMessageFragment = graphql(`
	fragment ThreadMessage on WhatsAppMessage {
		id
		externalId
		direction
		content
		contentType
		sentAt
		status
		statusDetail
		media {
			status
			mimeType
			filename
			fileSize
			voice
			animated
			downloadPath
		}
	}
`)

export const sendMessageMutation = graphql(`
	mutation WhatsAppSendMessage($conversationId: UUID!, $content: String!) {
		whatsAppSendMessage(conversationId: $conversationId, content: $content) {
			...ThreadMessage
		}
	}
`)

export const messageReceivedSubscription = graphql(`
	subscription WhatsAppMessageReceived($conversationId: UUID!) {
		whatsAppMessageReceived(conversationId: $conversationId) {
			...ThreadMessage
		}
	}
`)

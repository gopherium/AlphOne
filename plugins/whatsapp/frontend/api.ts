// SPDX-License-Identifier: AGPL-3.0-or-later

import { z } from 'zod'

const conversationSchema = z.object({
	id: z.string(),
	contact_id: z.string(),
	contact_name: z.string(),
	external_id: z.string(),
	status: z.string(),
	last_activity_at: z.coerce.date(),
})

export type Conversation = z.infer<typeof conversationSchema>

const messageSchema = z.object({
	id: z.string(),
	external_id: z.string(),
	direction: z.string(),
	content: z.string(),
	content_type: z.string(),
	sent_at: z.coerce.date(),
})

export type Message = z.infer<typeof messageSchema>

/**
 * Fetches all WhatsApp conversations from the backend API.
 * @returns A promise resolving to the parsed list of conversations.
 */
export async function fetchConversations(): Promise<Conversation[]> {
	const response = await fetch('/api/plugins/whatsapp/conversations')
	if (!response.ok) {
		throw new Error(`listing conversations failed with status ${response.status}`)
	}
	return z.array(conversationSchema).parse(await response.json())
}

/**
 * Fetches the messages belonging to a given conversation from the backend API.
 * @param conversationId - The identifier of the conversation to load messages for.
 * @returns A promise resolving to the parsed list of messages.
 */
export async function fetchMessages(
	conversationId: string,
): Promise<Message[]> {
	const response = await fetch(
		`/api/plugins/whatsapp/conversations/${conversationId}/messages`,
	)
	if (!response.ok) {
		throw new Error(`listing messages failed with status ${response.status}`)
	}
	return z.array(messageSchema).parse(await response.json())
}

/**
 * Sends a text message to the given conversation via the backend API.
 * @param conversationId - The identifier of the conversation to send the message to.
 * @param content - The text content of the message to send.
 * @returns A promise resolving to the parsed message created by the backend.
 */
export async function sendMessage(
	conversationId: string,
	content: string,
): Promise<Message> {
	const response = await fetch(
		`/api/plugins/whatsapp/conversations/${conversationId}/messages`,
		{
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ content }),
		},
	)
	if (!response.ok) {
		throw new Error(`sending message failed with status ${response.status}`)
	}
	return messageSchema.parse(await response.json())
}

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

export async function fetchConversations(): Promise<Conversation[]> {
	const response = await fetch('/api/plugins/whatsapp/conversations')
	if (!response.ok) {
		throw new Error(`listing conversations failed with status ${response.status}`)
	}
	return z.array(conversationSchema).parse(await response.json())
}

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

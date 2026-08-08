// SPDX-License-Identifier: AGPL-3.0-or-later

import { z } from 'zod'

const mediaSchema = z.object({
	status: z.string(),
	mime_type: z.string(),
	filename: z.string().nullable(),
	file_size: z.number().nullable(),
	voice: z.boolean(),
	animated: z.boolean(),
})

export type MessageMedia = z.infer<typeof mediaSchema>

const messageSchema = z.object({
	id: z.string(),
	external_id: z.string(),
	direction: z.enum(['inbound', 'outbound']),
	content: z.string(),
	content_type: z.string(),
	sent_at: z.coerce.date(),
	status: z.string().nullish(),
	status_detail: z.string().nullish(),
	media: mediaSchema.nullish(),
})

export type Message = z.infer<typeof messageSchema>

/**
 * Builds the download URL for a message's stored media blob.
 * @param conversationId - The conversation owning the message.
 * @param messageId - The message whose blob to address.
 * @returns The media endpoint URL.
 */
export function mediaURL(conversationId: string, messageId: string): string {
	return `/api/plugins/whatsapp/conversations/${conversationId}/messages/${messageId}/media`
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
 * SendFailedError reports a rejected reply, carrying the Graph error code
 * when the backend surfaced one.
 */
export class SendFailedError extends Error {
	readonly code: number | null

	/**
	 * Builds the error from the response status and any surfaced Graph code.
	 * @param status - The HTTP status of the failed send.
	 * @param code - The Graph error code, or null when none was surfaced.
	 */
	constructor(status: number, code: number | null) {
		super(`sending message failed with status ${status}`)
		this.name = 'SendFailedError'
		this.code = code
	}
}

const sendFailureSchema = z.object({ code: z.number() })

/**
 * Reads the Graph failure code from a rejected send response, if the
 * backend surfaced one.
 * @param response - The non-ok send response.
 * @returns The Graph error code, or null.
 */
async function failureCode(response: Response): Promise<number | null> {
	try {
		const parsed = sendFailureSchema.safeParse(await response.json())
		return parsed.success ? parsed.data.code : null
	} catch {
		return null
	}
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
		throw new SendFailedError(response.status, await failureCode(response))
	}
	return messageSchema.parse(await response.json())
}

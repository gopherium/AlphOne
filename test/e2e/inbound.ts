// SPDX-License-Identifier: AGPL-3.0-or-later

import { createHmac } from 'node:crypto'

import type { APIRequestContext } from '@playwright/test'

import { whatsappAppSecret } from './env'

/**
 * Builds the webhook body for an inbound text message.
 * @param waId - The sender's WhatsApp id.
 * @param name - The sender's profile name.
 * @param messageId - The message's external id.
 * @param text - The message body.
 * @returns The serialized webhook payload.
 */
function inboundTextPayload(
	waId: string,
	name: string,
	messageId: string,
	text: string,
): string {
	return JSON.stringify({
		entry: [
			{
				changes: [
					{
						value: {
							contacts: [{ wa_id: waId, profile: { name } }],
							messages: [
								{
									from: waId,
									id: messageId,
									timestamp: String(Math.floor(Date.now() / 1000)),
									type: 'text',
									text: { body: text },
								},
							],
						},
					},
				],
			},
		],
	})
}

/**
 * Returns the signature header value for a webhook body.
 * @param body - The serialized webhook payload.
 * @returns The X-Hub-Signature-256 value.
 */
function sign(body: string): string {
	return `sha256=${createHmac('sha256', whatsappAppSecret).update(body).digest('hex')}`
}

/**
 * Delivers an inbound text message through the WhatsApp webhook.
 * @param request - The Playwright request context.
 * @param waId - The sender's WhatsApp id.
 * @param name - The sender's profile name.
 * @param text - The message body.
 */
export async function deliverInboundText(
	request: APIRequestContext,
	waId: string,
	name: string,
	text: string,
): Promise<void> {
	const body = inboundTextPayload(waId, name, `wamid.e2e.${waId}`, text)
	const response = await request.post('/api/plugins/whatsapp/webhook', {
		headers: {
			'Content-Type': 'application/json',
			'X-Hub-Signature-256': sign(body),
		},
		data: body,
	})
	if (response.status() !== 200) {
		throw new Error(`the webhook answered ${response.status()}`)
	}
}

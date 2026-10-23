// SPDX-License-Identifier: AGPL-3.0-or-later

import { createHmac } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { parseEnv } from 'node:util'

import type { APIRequestContext } from '@playwright/test'

import { repoRoot } from './env'

/**
 * Reads the app secret the WhatsApp plugin's e2e settings hand the server.
 * @returns The app secret.
 */
function appSecret(): string {
	const settings = parseEnv(readFileSync(join(repoRoot, 'plugins', 'whatsapp', 'e2e.env'), 'utf8'))
	const secret = settings.ALPHONE_WHATSAPP_APP_SECRET
	if (!secret) {
		throw new Error('plugins/whatsapp/e2e.env names no ALPHONE_WHATSAPP_APP_SECRET')
	}
	return secret
}

/**
 * Builds the webhook body for an inbound text message.
 * @param waId - The sender's WhatsApp id.
 * @param name - The sender's profile name.
 * @param messageId - The message's external id.
 * @param text - The message body.
 * @param phoneNumberID - The receiving number's id, omitted for the install's own number.
 * @returns The serialized webhook payload.
 */
export function inboundTextPayload(
	waId: string,
	name: string,
	messageId: string,
	text: string,
	phoneNumberID?: string,
): string {
	return JSON.stringify({
		entry: [
			{
				changes: [
					{
						value: {
							...(phoneNumberID ? { metadata: { phone_number_id: phoneNumberID } } : {}),
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
export function sign(body: string): string {
	return `sha256=${createHmac('sha256', appSecret()).update(body).digest('hex')}`
}

/**
 * Delivers an inbound text message through the WhatsApp webhook.
 * @param request - The Playwright request context.
 * @param waId - The sender's WhatsApp id.
 * @param name - The sender's profile name.
 * @param text - The message body.
 * @param phoneNumberID - The receiving number's id, omitted for the install's own number.
 */
export async function deliverInboundText(
	request: APIRequestContext,
	waId: string,
	name: string,
	text: string,
	phoneNumberID?: string,
): Promise<void> {
	const body = inboundTextPayload(waId, name, `wamid.e2e.${waId}`, text, phoneNumberID)
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

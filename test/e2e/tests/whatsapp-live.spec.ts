// SPDX-License-Identifier: AGPL-3.0-or-later

import { createHmac } from 'node:crypto'

import { expect, test } from '@playwright/test'

import { whatsappAppSecret } from '../env'

function inboundTextPayload(
	waId: string,
	name: string,
	messageId: string,
	text: string,
) {
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

function sign(body: string) {
	return `sha256=${createHmac('sha256', whatsappAppSecret).update(body).digest('hex')}`
}

test('shows an inbound conversation pushed over the live stream', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const waId = `1555${stamp}`
	const contactName = `Ada ${stamp}`
	const body = inboundTextPayload(waId, contactName, `wamid.e2e.${stamp}`, 'hola')

	const stream = page.waitForResponse((response) =>
		response.url().includes('/api/plugins/whatsapp/events'),
	)
	await page.goto('/whatsapp')
	await stream
	await expect(page.getByText(contactName)).toBeHidden()

	const delivered = await request.post('/api/plugins/whatsapp/webhook', {
		headers: {
			'Content-Type': 'application/json',
			'X-Hub-Signature-256': sign(body),
		},
		data: body,
	})

	expect(delivered.status()).toBe(200)
	await expect(page.getByText(contactName)).toBeVisible()
})

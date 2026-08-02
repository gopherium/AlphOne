// SPDX-License-Identifier: AGPL-3.0-or-later

import { createHmac } from 'node:crypto'

import { expect, test } from '@playwright/test'

import { whatsappAppSecret } from '../env'

function inboundTextPayload(waId: string, name: string, messageId: string, text: string) {
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

function canvasGeometry(page: import('@playwright/test').Page) {
	return page.evaluate(() => {
		const canvas = document.querySelector('.alphone-layout__canvas')
		if (canvas === null) {
			throw new Error('the layout rendered no canvas')
		}
		const style = getComputedStyle(canvas)
		return {
			paddingTop: style.paddingTop,
			paddingLeft: style.paddingLeft,
			clientWidth: canvas.clientWidth,
			scrollWidth: canvas.scrollWidth,
			clientHeight: canvas.clientHeight,
			scrollHeight: canvas.scrollHeight,
		}
	})
}

test('a full bleed screen fills the canvas without escaping it', async ({ page, request }) => {
	const stamp = Date.now()
	const waId = `1777${stamp}`
	const contactName = `Maria ${stamp}`
	const body = inboundTextPayload(waId, contactName, `wamid.e2e.bleed.${stamp}`, 'hello')

	const stream = page.waitForResponse((response) =>
		response.url().includes('/api/plugins/whatsapp/events'),
	)
	await page.goto('/whatsapp')
	await stream

	const delivered = await request.post('/api/plugins/whatsapp/webhook', {
		headers: {
			'Content-Type': 'application/json',
			'X-Hub-Signature-256': sign(body),
		},
		data: body,
	})
	expect(delivered.status()).toBe(200)

	await expect(page.getByText(contactName)).toBeVisible()
	await page.getByText(contactName).click()
	await expect(page.getByRole('log', { name: 'Messages' })).toBeVisible()

	const geometry = await canvasGeometry(page)

	expect(geometry.paddingTop).toBe('0px')
	expect(geometry.paddingLeft).toBe('0px')
	expect(geometry.scrollWidth).toBe(geometry.clientWidth)
	expect(geometry.scrollHeight).toBe(geometry.clientHeight)
})

test('a normal screen keeps the canvas padding', async ({ page }) => {
	await page.goto('/tasks')
	await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

	const geometry = await canvasGeometry(page)

	expect(geometry.paddingTop).toBe('24px')
	expect(geometry.paddingLeft).toBe('24px')
})

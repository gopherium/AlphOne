// SPDX-License-Identifier: AGPL-3.0-or-later

import { createHash, createHmac } from 'node:crypto'
import { createServer } from 'node:http'
import type { Server } from 'node:http'

import { expect, test } from '@playwright/test'

import { whatsappAppSecret } from '../env'

const mockGraphPort = 4791
const metadataDelayMs = 3000
const imageBytes = Buffer.from(`e2e-image-bytes-${Date.now()}`)
const imageSha = createHash('sha256').update(imageBytes).digest('base64')

function startMockGraph(): Promise<Server> {
	const server = createServer((request, response) => {
		const url = new URL(request.url ?? '/', `http://127.0.0.1:${mockGraphPort}`)
		if (url.pathname.startsWith('/binary/')) {
			response.writeHead(200, { 'Content-Type': 'image/jpeg' })
			response.end(imageBytes)
			return
		}
		const mediaID = url.pathname.slice(1)
		setTimeout(() => {
			response.writeHead(200, { 'Content-Type': 'application/json' })
			response.end(
				JSON.stringify({
					url: `http://127.0.0.1:${mockGraphPort}/binary/${mediaID}`,
					mime_type: 'image/jpeg',
					sha256: imageSha,
					file_size: imageBytes.length,
					id: mediaID,
				}),
			)
		}, metadataDelayMs)
	})
	return new Promise((resolve) => {
		server.listen(mockGraphPort, '127.0.0.1', () => resolve(server))
	})
}

function inboundImagePayload(
	waId: string,
	name: string,
	messageId: string,
	mediaID: string,
	caption: string,
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
									type: 'image',
									image: {
										id: mediaID,
										mime_type: 'image/jpeg',
										sha256: imageSha,
										caption,
									},
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

let mockGraph: Server

test.beforeAll(async () => {
	mockGraph = await startMockGraph()
})

test.afterAll(async () => {
	await new Promise((resolve) => mockGraph.close(resolve))
})

test('delivers an inbound photo from the webhook to the thread', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const waId = `1666${stamp}`
	const contactName = `Frida ${stamp}`
	const caption = `una foto ${stamp}`
	const body = inboundImagePayload(
		waId,
		contactName,
		`wamid.e2e.media.${stamp}`,
		`MEDIA-${stamp}`,
		caption,
	)

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

	const log = page.getByRole('log', { name: 'Messages' })
	await expect(log.getByText('Downloading…')).toBeVisible()

	const image = page.getByAltText('Photo')
	await expect(image).toBeVisible({ timeout: 15_000 })
	await expect(image).toHaveAttribute('src', /^blob:/)
	await expect(log.getByText(caption)).toBeVisible()
})

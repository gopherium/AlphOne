// SPDX-License-Identifier: AGPL-3.0-or-later

import { createHash } from 'node:crypto'
import { createServer } from 'node:http'
import type { Server } from 'node:http'

import { expect, test } from '@playwright/test'

import { sign } from '../../../test/e2e/inbound'
import { subscribed } from '../../../test/e2e/subscription'

const mockGraphPort = 4791
const imageBytes = Buffer.from(`e2e-image-bytes-${Date.now()}`)
const imageSha = createHash('sha256').update(imageBytes).digest('base64')

/** MockGraph is the Meta stand in beside the lever releasing its metadata answer. */
type MockGraph = { server: Server; releaseMetadata: () => void }

/**
 * Starts a stand in for Meta, holding its metadata answer until released.
 * @returns The listening server beside the lever releasing the held answer.
 */
function startMockGraph(): Promise<MockGraph> {
	let releaseMetadata: () => void = () => {}
	const held = new Promise<void>((resolve) => {
		releaseMetadata = resolve
	})
	const server = createServer((request, response) => {
		const url = new URL(request.url ?? '/', `http://127.0.0.1:${mockGraphPort}`)
		if (url.pathname.startsWith('/binary/')) {
			response.writeHead(200, { 'Content-Type': 'image/jpeg' })
			response.end(imageBytes)
			return
		}
		const mediaID = url.pathname.slice(1)
		void held.then(() => {
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
		})
	})
	return new Promise((resolve) => {
		server.listen(mockGraphPort, '127.0.0.1', () =>
			resolve({ server, releaseMetadata }),
		)
	})
}

/** InboundImage is one image a sender posts, as the webhook names its parts. */
type InboundImage = { waId: string; name: string; messageId: string; mediaID: string; caption: string }

/**
 * Builds the webhook body Meta posts for an inbound image.
 * @param image - The sender, the message and the media the image carries.
 * @returns The serialized webhook body.
 */
function inboundImagePayload({ waId, name, messageId, mediaID, caption }: InboundImage) {
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


let mockGraph: MockGraph

test.beforeAll(async () => {
	mockGraph = await startMockGraph()
})

test.afterAll(async () => {
	mockGraph.releaseMetadata()
	await new Promise((resolve) => mockGraph.server.close(resolve))
})

test('delivers an inbound photo from the webhook to the thread', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const waId = `1666${stamp}`
	const contactName = `Frida ${stamp}`
	const caption = `a photo ${stamp}`
	const body = inboundImagePayload({
		waId,
		name: contactName,
		messageId: `wamid.e2e.media.${stamp}`,
		mediaID: `MEDIA-${stamp}`,
		caption,
	})

	const stream = subscribed(page, 'whatsAppConversationEvent')
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

	mockGraph.releaseMetadata()

	const image = page.getByAltText('Photo')
	await expect(image).toBeVisible({ timeout: 15_000 })
	await expect(image).toHaveAttribute('src', /^blob:/)
	await expect(log.getByText(caption)).toBeVisible()
})

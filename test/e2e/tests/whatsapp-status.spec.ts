// SPDX-License-Identifier: AGPL-3.0-or-later

import { createHmac } from 'node:crypto'
import { createServer } from 'node:http'
import type { Server } from 'node:http'

import { expect, test } from '@playwright/test'

import { whatsappAppSecret } from '../env'

const mockGraphPort = 4791
const stamp = Date.now()
const sentWamid = `wamid.e2e.status.out.${stamp}`

function startMockGraph(): Promise<Server> {
	const server = createServer((request, response) => {
		request.resume()
		request.on('end', () => {
			response.writeHead(200, { 'Content-Type': 'application/json' })
			response.end(JSON.stringify({ messages: [{ id: sentWamid }] }))
		})
	})
	return new Promise((resolve) => {
		server.listen(mockGraphPort, '127.0.0.1', () => resolve(server))
	})
}

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

function statusPayload(wamid: string, status: string) {
	return JSON.stringify({
		entry: [
			{
				changes: [
					{
						value: {
							statuses: [
								{
									id: wamid,
									status,
									timestamp: String(Math.floor(Date.now() / 1000)),
									recipient_id: '184467235',
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

test('progresses delivery ticks on an outbound reply', async ({
	page,
	request,
}) => {
	const waId = `1777${stamp}`
	const contactName = `Tania ${stamp}`
	const inbound = inboundTextPayload(
		waId,
		contactName,
		`wamid.e2e.status.in.${stamp}`,
		'hola',
	)

	const stream = page.waitForResponse((response) =>
		response.url().includes('/api/plugins/whatsapp/events'),
	)
	await page.goto('/whatsapp')
	await stream

	const delivered = await request.post('/api/plugins/whatsapp/webhook', {
		headers: {
			'Content-Type': 'application/json',
			'X-Hub-Signature-256': sign(inbound),
		},
		data: inbound,
	})
	expect(delivered.status()).toBe(200)
	await page.getByText(contactName).click()

	const reply = `listo ${stamp}`
	await page.getByRole('textbox', { name: 'Reply' }).fill(reply)
	await page.getByRole('button', { name: 'Send' }).click()
	const log = page.getByRole('log', { name: 'Messages' })
	await expect(log.getByText(reply)).toBeVisible()
	const ticks = page.locator('.alphone-message__ticks')
	await expect(ticks).toHaveCount(0)

	const deliveredBody = statusPayload(sentWamid, 'delivered')
	const deliveredStatus = await request.post('/api/plugins/whatsapp/webhook', {
		headers: {
			'Content-Type': 'application/json',
			'X-Hub-Signature-256': sign(deliveredBody),
		},
		data: deliveredBody,
	})
	expect(deliveredStatus.status()).toBe(200)
	await expect(ticks).toContainText('✓✓')
	await expect(ticks).not.toHaveClass(/alphone-message__ticks--read/)

	const readBody = statusPayload(sentWamid, 'read')
	const readStatus = await request.post('/api/plugins/whatsapp/webhook', {
		headers: {
			'Content-Type': 'application/json',
			'X-Hub-Signature-256': sign(readBody),
		},
		data: readBody,
	})
	expect(readStatus.status()).toBe(200)
	await expect(ticks).toHaveClass(/alphone-message__ticks--read/)
})

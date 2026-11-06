// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { sign } from '../../../test/e2e/inbound'

/** chatLength is how many messages the chat holds, past the default page of 50. */
const chatLength = 60

/**
 * Builds one webhook body carrying a whole chat from one sender, a second apart, oldest first.
 * @param waId - The sender's WhatsApp id.
 * @param name - The sender's profile name.
 * @param count - How many messages the chat holds.
 * @returns The serialized webhook body.
 */
function longChatPayload(waId: string, name: string, count: number): string {
	const start = Math.floor(Date.now() / 1000) - count - 60
	const messages = Array.from({ length: count }, (_, index) => ({
		from: waId,
		id: `wamid.e2e.long.${waId}.${index + 1}`,
		timestamp: String(start + index),
		type: 'text',
		text: { body: `Message ${index + 1}` },
	}))
	return JSON.stringify({
		entry: [{ changes: [{ value: { contacts: [{ wa_id: waId, profile: { name } }], messages } }] }],
	})
}

test("a long chat opens on its newest message and reaches its oldest, keeping the reader's place", async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const waId = `1444${stamp}`
	const contactName = `Edsger ${stamp}`
	const body = longChatPayload(waId, contactName, chatLength)
	const delivered = await request.post('/api/plugins/whatsapp/webhook', {
		headers: { 'Content-Type': 'application/json', 'X-Hub-Signature-256': sign(body) },
		data: body,
	})
	expect(delivered.status()).toBe(200)

	await page.goto('/whatsapp')
	await page.getByText(contactName).click()
	const log = page.getByRole('log', { name: 'Messages' })
	await expect(log.getByText(`Message ${chatLength}`, { exact: true })).toBeInViewport()
	await expect(log.getByText('Message 1', { exact: true })).toHaveCount(0)

	const older = page.getByRole('button', { name: 'Show older messages' })
	const topRow = log.locator('.alphone-message__content').first()
	await log.evaluate((element) => {
		element.scrollTop = 0
	})
	const firstText = (await topRow.textContent()) ?? ''
	const firstShown = log.getByText(firstText, { exact: true })
	await expect(firstShown).toBeInViewport()
	const before = await firstShown.boundingBox()
	await older.click()
	await expect(topRow).not.toHaveText(firstText)
	const after = await firstShown.boundingBox()

	expect(before).not.toBeNull()
	expect(after).not.toBeNull()
	expect(Math.abs((after?.y ?? 0) - (before?.y ?? 0))).toBeLessThan(2)
	for (let clicks = 1; clicks < chatLength && (await older.isVisible()); clicks += 1) {
		const shownFirst = (await topRow.textContent()) ?? ''
		await older.click()
		await expect(topRow).not.toHaveText(shownFirst)
	}
	await expect(topRow).toHaveText('Message 1')
	await expect(older).toBeHidden()
})

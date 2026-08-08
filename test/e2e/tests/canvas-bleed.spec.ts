// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { deliverInboundText } from '../inbound'
import { subscribed } from '../subscription'

function canvasGeometry(page: import('@playwright/test').Page) {
	return page.evaluate(() => {
		const canvas = document.querySelector('.godmin-layout__canvas')
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

	const stream = subscribed(page, 'whatsAppConversationEvent')
	await page.goto('/whatsapp')
	await stream

	await deliverInboundText(request, waId, contactName, 'hello')

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

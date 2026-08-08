// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { deliverInboundText } from '../inbound'
import { subscribed } from '../subscription'

test('shows an inbound conversation pushed over the live stream', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const waId = `1555${stamp}`
	const contactName = `Ada ${stamp}`

	const stream = subscribed(page, 'whatsAppConversationEvent')
	await page.goto('/whatsapp')
	await stream
	await expect(page.getByText(contactName)).toBeHidden()

	await deliverInboundText(request, waId, contactName, 'hello')

	await expect(page.getByText(contactName)).toBeVisible()
})

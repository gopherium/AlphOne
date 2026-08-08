// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { subscribed } from '../subscription'

test('shows a task created through the API without a reload', async ({
	page,
	request,
}) => {
	const title = `Booked by the engine ${Date.now()}`
	const dueOn = new Date().toISOString().slice(0, 10)

	const stream = subscribed(page, 'coreEvent')
	await page.goto('/')
	await stream
	await expect(page.getByRole('listitem', { name: title })).toBeHidden()

	const created = await request.post('/api/tasks', {
		data: { title, due_on: dueOn },
	})
	expect(created.status()).toBe(201)

	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
})

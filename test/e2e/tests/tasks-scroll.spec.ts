// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

test('a day taller than the screen scrolls to its last task', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const dueOn = new Date().toISOString().slice(0, 10)
	const created: string[] = []
	try {
		for (let i = 0; i < 15; i++) {
			const response = await request.post('/api/tasks', {
				data: { title: `Scroll filler ${stamp} ${i}`, due_on: dueOn },
			})
			expect(response.status()).toBe(201)
			created.push(((await response.json()) as { id: string }).id)
		}

		await page.goto('/')
		await expect(
			page.getByRole('listitem', { name: `Scroll filler ${stamp} 0` }),
		).toBeVisible()

		const canvas = page.locator('.alphone-layout__canvas')
		await canvas.hover()
		await page.mouse.wheel(0, 2000)

		await expect
			.poll(() => canvas.evaluate((node) => node.scrollTop))
			.toBeGreaterThan(0)
	} finally {
		for (const id of created) {
			await request.patch(`/api/tasks/${id}`, { data: { due_on: '2124-01-01' } })
		}
	}
})

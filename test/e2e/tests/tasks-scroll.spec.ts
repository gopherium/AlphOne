// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { createTask, rescheduleTask } from '../graph'

test('a day taller than the screen scrolls to its last task', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const dueOn = new Date().toISOString().slice(0, 10)
	const created: string[] = []
	try {
		for (let i = 0; i < 15; i++) {
			const task = await createTask(request, { title: `Scroll filler ${stamp} ${i}`, dueOn })
			created.push(task.id)
		}

		await page.goto('/')
		await expect(
			page.getByRole('listitem', { name: `Scroll filler ${stamp} 0` }),
		).toBeVisible()

		const canvas = page.locator('.godmin-layout__canvas')
		await canvas.hover()
		await page.mouse.wheel(0, 2000)

		await expect
			.poll(() => canvas.evaluate((node) => node.scrollTop))
			.toBeGreaterThan(0)
	} finally {
		for (const id of created) {
			await rescheduleTask(request, id, '2124-01-01')
		}
	}
})

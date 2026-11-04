// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Locator } from '@playwright/test'

/** The most two lined up edges may differ by, in pixels. */
const linedUp = 2

/** Box is the part of a bounding box the layout checks read. */
type Box = { x: number; y: number; height: number }

/**
 * Returns where a control sits on the page.
 * @param control - The control to read.
 * @returns Its box.
 */
async function boxOf(control: Locator): Promise<Box> {
	const box = await control.boundingBox()
	expect(box).not.toBeNull()
	return box as Box
}

for (const width of [1920, 1280]) {
	test(`keeps the controls of a form row on one line while a message shows under one at ${width}`, async ({ page }) => {
		await page.setViewportSize({ width, height: 900 })
		await page.goto('/tasks?date=2030-01-01')
		await page.route('**/api/graphql', async (route) => {
			if (route.request().postData()?.includes('CreateTask')) {
				await route.fulfill({
					json: {
						data: null,
						errors: [{ message: 'task: empty title', extensions: { code: 'VALIDATION', reason: 'task_title_required' } }],
					},
				})
				return
			}
			await route.continue()
		})

		const title = page.getByLabel('Task title', { exact: true })
		const add = page.getByRole('button', { name: 'Add task' })
		const before = await boxOf(title)
		await title.fill('Call Maria Perez')
		await add.click()

		const message = page.locator('form').getByText('A task needs a title.')
		await expect(message).toBeVisible()
		await expect(title).toBeFocused()
		const field = await boxOf(title)
		expect(field.y).toBeCloseTo(before.y, 0)
		const under = await boxOf(message)
		expect(under.y).toBeGreaterThanOrEqual(field.y + field.height)
		expect(Math.abs(under.x - field.x)).toBeLessThanOrEqual(linedUp)
		const others = [
			page.getByLabel('Due date', { exact: true }),
			page.getByLabel('Time', { exact: true }),
			page.getByRole('combobox', { name: 'Priority' }),
			add,
		]
		for (const control of others) {
			expect(Math.abs((await boxOf(control)).y - field.y)).toBeLessThanOrEqual(linedUp)
		}
	})
}

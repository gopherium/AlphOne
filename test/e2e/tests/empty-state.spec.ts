// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

test('an empty state sits centered in the canvas', async ({ page }) => {
	await page.goto('/tasks?date=2030-01-01')
	await expect(page.getByText('Nothing due today.')).toBeVisible()

	const geometry = await page.evaluate(() => {
		const canvas = document.querySelector('.alphone-layout__canvas')
		const empty = document.querySelector('.alphone-empty')
		if (canvas === null || empty === null) {
			throw new Error('the screen rendered no canvas or no empty state')
		}
		const style = getComputedStyle(canvas)
		const box = canvas.getBoundingClientRect()
		const contentLeft = box.left + parseFloat(style.paddingLeft)
		const contentRight = box.right - parseFloat(style.paddingRight)
		const emptyBox = empty.getBoundingClientRect()
		return {
			canvasCenter: (contentLeft + contentRight) / 2,
			emptyCenter: emptyBox.left + emptyBox.width / 2,
		}
	})

	expect(geometry.emptyCenter).toBeCloseTo(geometry.canvasCenter, 0)
})

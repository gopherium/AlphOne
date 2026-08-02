// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

test('the page template spans the full canvas so actions pin to the right edge', async ({
	page,
}) => {
	await page.goto('/tasks')
	await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

	const geometry = await page.evaluate(() => {
		const canvas = document.querySelector('.godmin-layout__canvas')
		const screen = document.querySelector('.godmin-page')
		if (canvas === null || screen === null) {
			throw new Error('the layout rendered no canvas or no page')
		}
		const style = getComputedStyle(canvas)
		const contentWidth =
			canvas.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
		return {
			contentWidth,
			pageWidth: screen.getBoundingClientRect().width,
		}
	})

	expect(geometry.pageWidth).toBeCloseTo(geometry.contentWidth, 0)
})

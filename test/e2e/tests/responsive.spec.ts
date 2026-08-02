// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

const phone = { width: 390, height: 844 }

test.describe('on a phone', () => {
	test.use({ viewport: phone })

	test('swaps the rail for a drawer and spends the width on content', async ({ page }) => {
		await page.goto('/tasks')
		await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

		await expect(page.locator('.alphone-layout__sidebar')).toHaveCount(0)

		const geometry = await page.evaluate(() => {
			const canvas = document.querySelector('.alphone-layout__canvas')
			if (canvas === null) {
				throw new Error('the layout rendered no canvas')
			}
			const style = getComputedStyle(canvas)
			return {
				canvasContentWidth:
					canvas.clientWidth -
					parseFloat(style.paddingLeft) -
					parseFloat(style.paddingRight),
				documentOverflows:
					document.documentElement.scrollWidth > document.documentElement.clientWidth,
			}
		})

		expect(geometry.documentOverflows).toBe(false)
		expect(geometry.canvasContentWidth).toBeGreaterThan(280)
	})

	test('reaches another section through the drawer', async ({ page }) => {
		await page.goto('/tasks')
		await page.getByRole('button', { name: 'Open navigation' }).click()

		const drawer = page.getByRole('dialog')
		await expect(drawer).toBeVisible()
		await drawer.getByRole('link', { name: 'Contacts' }).click()

		await expect(page.getByRole('heading', { name: 'Contacts' })).toBeVisible()
		await expect(drawer).toBeHidden()
	})
})

test('keeps the rail on a desktop viewport', async ({ page }) => {
	await page.goto('/tasks')
	await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

	await expect(page.locator('.alphone-layout__sidebar')).toBeVisible()
	await expect(page.getByRole('button', { name: 'Open navigation' })).toHaveCount(0)
})

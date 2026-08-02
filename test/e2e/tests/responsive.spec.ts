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

	test('gives a task row title room instead of squeezing it past its controls', async ({
		page,
		request,
	}) => {
		const title = `Approve the pricing before the renewal ${Date.now()}`
		const created = await request.post('/api/tasks', {
			data: { title, due_on: new Date().toISOString().slice(0, 10) },
		})
		expect(created.status()).toBe(201)

		await page.goto('/tasks')
		await expect(page.getByRole('listitem', { name: title })).toBeVisible()

		const geometry = await page.evaluate((name) => {
			const row = [...document.querySelectorAll('.alphone-tasks__row')].find(
				(candidate) => candidate.getAttribute('aria-label') === name,
			)
			const title = row?.querySelector('.alphone-tasks__title')
			if (!row || !title) {
				throw new Error('the screen rendered no task row')
			}
			const rowBox = row.getBoundingClientRect()
			const titleBox = title.getBoundingClientRect()
			const line = parseFloat(getComputedStyle(title).lineHeight)
			return {
				titleShare: titleBox.width / rowBox.width,
				titleLines: Math.round(titleBox.height / line),
			}
		}, title)

		expect(geometry.titleShare).toBeGreaterThan(0.5)
		expect(geometry.titleLines).toBeLessThanOrEqual(2)
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

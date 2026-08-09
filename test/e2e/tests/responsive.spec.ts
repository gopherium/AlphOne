// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { createTask } from '../graph'
import { deliverInboundText } from '../inbound'

const phone = { width: 390, height: 844 }

test.describe('on a phone', () => {
	test.use({ viewport: phone })

	test('swaps the rail for a drawer and spends the width on content', async ({ page }) => {
		await page.goto('/tasks')
		await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

		await expect(page.locator('.godmin-layout__rail')).toHaveCount(0)

		const geometry = await page.evaluate(() => {
			const canvas = document.querySelector('.godmin-layout__canvas')
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

	test('spends its pixels on content rather than canvas chrome', async ({ page }) => {
		await page.goto('/tasks')
		await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

		const canvas = await page.evaluate(() => {
			const element = document.querySelector('.godmin-layout__canvas')
			if (element === null) {
				throw new Error('the layout rendered no canvas')
			}
			const style = getComputedStyle(element)
			return {
				margin: style.marginLeft,
				padding: style.paddingLeft,
				radius: style.borderTopLeftRadius,
				contentWidth:
					element.clientWidth -
					parseFloat(style.paddingLeft) -
					parseFloat(style.paddingRight),
			}
		})

		expect(canvas.margin).toBe('0px')
		expect(canvas.padding).toBe('16px')
		expect(canvas.radius).toBe('0px')
		expect(canvas.contentWidth).toBeGreaterThan(350)
	})

	test('gives a task row title room instead of squeezing it past its controls', async ({
		page,
		request,
	}) => {
		const title = `Approve the pricing before the renewal ${Date.now()}`
		await createTask(request, { title, dueOn: new Date().toISOString().slice(0, 10) })

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

	test('fits a conversation thread edge to edge with a reachable composer', async ({
		page,
		request,
	}) => {
		const stamp = Date.now()
		const contactName = `Maria ${stamp}`
		await deliverInboundText(request, `1888${stamp}`, contactName, 'hello')

		await page.goto('/whatsapp')
		// The conversation list is this section's sidebar, so on a phone it is
		// reached through the drawer.
		await page.getByRole('button', { name: 'Open navigation' }).click()
		await page.getByRole('dialog').getByText(contactName).click()
		await expect(page.getByRole('log', { name: 'Messages' })).toBeVisible()

		const geometry = await page.evaluate(() => {
			const canvas = document.querySelector('.godmin-layout__canvas')
			const header = document.querySelector('.alphone-thread__header')
			const log = document.querySelector('.alphone-thread__log')
			const composer = document.querySelector('.alphone-composer')
			if (!canvas || !header || !log || !composer) {
				throw new Error('the thread did not render')
			}
			const box = canvas.getBoundingClientRect()
			return {
				logPadding: getComputedStyle(log).paddingLeft,
				composerInView: composer.getBoundingClientRect().bottom <= window.innerHeight + 1,
				headerSpansCanvas:
					Math.round(header.getBoundingClientRect().width) === Math.round(box.width),
				canvasOverflows: canvas.scrollWidth > canvas.clientWidth,
			}
		})

		expect(geometry.logPadding).toBe('16px')
		expect(geometry.headerSpansCanvas).toBe(true)
		expect(geometry.composerInView).toBe(true)
		expect(geometry.canvasOverflows).toBe(false)
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

	await expect(page.locator('.godmin-layout__rail')).toBeVisible()
	await expect(page.getByRole('button', { name: 'Open navigation' })).toHaveCount(0)
})

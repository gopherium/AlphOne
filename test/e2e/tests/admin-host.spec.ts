// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

test('paints the page from design tokens', async ({ page }) => {
	await page.goto('/')
	await expect(page.locator('.godmin-layout')).toBeVisible()

	const painted = await page.evaluate(() => {
		const root = getComputedStyle(document.documentElement)
		const probe = document.createElement('div')
		probe.style.backgroundColor = 'var(--wpds-color-background-surface-neutral)'
		probe.style.color = 'var(--wpds-color-foreground-content-neutral)'
		probe.style.fontFamily = 'var(--wpds-typography-font-family-body)'
		document.body.append(probe)
		const token = getComputedStyle(probe)
		const expected = {
			backgroundColor: token.backgroundColor,
			color: token.color,
			fontFamily: token.fontFamily,
		}
		probe.remove()
		const body = getComputedStyle(document.body)
		return {
			expected,
			actual: {
				backgroundColor: body.backgroundColor,
				color: body.color,
				fontFamily: body.fontFamily,
			},
			declared: [
				root.getPropertyValue('--wpds-color-background-surface-neutral'),
				root.getPropertyValue('--wpds-color-foreground-content-neutral'),
				root.getPropertyValue('--wpds-typography-font-family-body'),
			],
			margin: body.margin,
			position: body.position,
		}
	})

	expect(painted.declared.filter((value) => value.trim() === '')).toEqual([])
	expect(painted.actual).toEqual(painted.expected)
	expect(painted.margin).toBe('0px')
	expect(painted.position).toBe('relative')
})

test('mounts the application inside an isolation context', async ({ page }) => {
	await page.goto('/')
	await expect(page.locator('.godmin-layout')).toBeVisible()

	const isolated = await page.evaluate(() => {
		let node = document.querySelector('.godmin-layout')?.parentElement
		while (node && node !== document.body) {
			if (getComputedStyle(node).isolation === 'isolate') {
				return true
			}
			node = node.parentElement
		}
		return false
	})

	expect(isolated).toBe(true)
})

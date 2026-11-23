// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Browser, BrowserContext, Page } from '@playwright/test'

import { authFile, baseURL } from '../env'
import { provisionUser } from '../provision'

/** phone is the viewport of a phone held upright. */
const phone = { width: 390, height: 844 }

/** desktop is the viewport of a wide screen. */
const desktop = { width: 1920, height: 1080 }

/** reader is the account that reads the form in Spanish, so the shared one keeps reading English. */
const reader = {
	email: `e2e-token-form-${Date.now()}@example.com`,
	name: 'Maria Perez',
	password: 'correct horse battery',
}

/** SignedIn is the browser state of an account that has signed in. */
type SignedIn = Awaited<ReturnType<BrowserContext['storageState']>>

/** spanish is the signed-in state of the reader once it reads Spanish. */
let spanish: SignedIn

/**
 * Signs the reader in and picks Spanish on the Language screen.
 * @param browser - The browser the reader's context opens in.
 * @returns The reader's signed-in state.
 */
async function signInInSpanish(browser: Browser): Promise<SignedIn> {
	const context = await browser.newContext({ baseURL, storageState: { cookies: [], origins: [] } })
	const page = await context.newPage()
	await page.goto('/')
	await page.getByLabel('Email').fill(reader.email)
	await page.getByLabel('Password').fill(reader.password)
	await page.getByRole('button', { name: 'Log in' }).click()
	await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

	await page.goto('/language')
	await page.getByRole('combobox', { name: 'Language' }).click()
	await page.getByRole('option', { name: 'es-ES' }).click()
	await page.getByRole('button', { name: 'Save' }).click()
	await expect(page.getByRole('status')).toBeVisible()

	const state = await context.storageState()
	await context.close()
	return state
}

/**
 * Opens the New token form as the Spanish reader in a window of one size.
 * @param browser - The browser the form opens in.
 * @param viewport - The size of the window.
 * @returns The page showing the form.
 */
async function openForm(browser: Browser, viewport: { width: number; height: number }): Promise<Page> {
	const context = await browser.newContext({ baseURL, storageState: spanish, viewport })
	const page = await context.newPage()
	await page.goto('/users/tokens/new')
	await expect(page.getByRole('heading', { level: 2, name: 'Token nuevo' })).toBeVisible()
	await expect(page.getByRole('checkbox', { name: 'Escribir correo electrónico' })).toBeVisible()
	return page
}

test.beforeAll(async ({ browser }) => {
	const admin = await browser.newContext({ baseURL, storageState: authFile })
	await provisionUser(await admin.newPage(), browser, reader)
	await admin.close()
	spanish = await signInInSpanish(browser)
})

test('keeps every area label and description beside its box on a phone', async ({ browser }) => {
	const page = await openForm(browser, phone)
	await expect(page.getByRole('checkbox', { name: 'Escribir correo electrónico' })).toHaveAccessibleDescription(/\S/)

	const strays = await page.getByRole('checkbox').evaluateAll((boxes) => {
		/**
		 * Lists every line of one label or description that starts left of its box's right edge.
		 * @param part - The label or description whose lines are measured.
		 * @param edge - The right edge of the box the part belongs to.
		 * @returns One message per stray line, or one message when the part shows no line of text.
		 */
		function strayLines(part: HTMLElement, edge: number): string[] {
			const walker = document.createTreeWalker(part, NodeFilter.SHOW_TEXT)
			const range = document.createRange()
			const lines: number[] = []
			for (let text = walker.nextNode(); text !== null; text = walker.nextNode()) {
				range.selectNodeContents(text)
				lines.push(...[...range.getClientRects()].map((line) => line.left))
			}
			if (lines.length === 0) {
				return [`${part.outerHTML}: shows no line of text`]
			}
			return lines
				.filter((left) => left < edge)
				.map((left) => `${part.textContent?.trim()}: a line starts at ${left}, left of the box edge at ${edge}`)
		}

		return boxes.flatMap((box) => {
			const labelledBy = box.getAttribute('aria-labelledby')
			const label = labelledBy === null ? box.closest('label') : document.getElementById(labelledBy.split(' ')[0])
			if (label === null) {
				return [`a box has no label: ${box.outerHTML}`]
			}
			const describedBy = (box.getAttribute('aria-describedby') ?? '').split(' ').filter((id) => id !== '')
			const edge = box.getBoundingClientRect().right
			return [label, ...describedBy.map((id) => document.getElementById(id))].flatMap((part) =>
				part === null ? [`${box.outerHTML}: names a description that is not on the page`] : strayLines(part, edge),
			)
		})
	})

	expect(strays).toEqual([])
	await page.context().close()
})

for (const viewport of [desktop, phone]) {
	test(`lines up every Write box in one column at ${viewport.width} wide`, async ({ browser }) => {
		const page = await openForm(browser, viewport)

		const lefts = await page
			.getByRole('checkbox', { name: /^Escribir / })
			.evaluateAll((boxes) => boxes.map((box) => box.getBoundingClientRect().left))

		expect(lefts.length).toBeGreaterThan(1)
		expect(Math.max(...lefts) - Math.min(...lefts), JSON.stringify(lefts)).toBeLessThanOrEqual(1)
		await page.context().close()
	})
}

// SPDX-License-Identifier: AGPL-3.0-or-later

import type { Browser, Page } from '@playwright/test'

import { linkFrom, startMailSink } from './mail'

/**
 * Invites one person and activates their account from the mailed link.
 * @param page - The signed-in administrator's page.
 * @param browser - The browser the activation opens a clean context in.
 * @param account - The address, name, and password the account ends up with.
 */
export async function provisionUser(
	page: Page,
	browser: Browser,
	account: { email: string; name: string; password: string },
): Promise<void> {
	const sink = await startMailSink()
	try {
		await page.goto('/users/new')
		await page.getByLabel('Email').fill(account.email)
		await page.getByLabel('Name').fill(account.name)
		await page.getByRole('button', { name: 'Send invitation' }).click()

		const body = await sink.waitFor(account.email)
		const context = await browser.newContext({
			storageState: { cookies: [], origins: [] },
		})
		const activation = await context.newPage()
		await activation.goto(linkFrom(body, '/activate'))
		await activation.getByLabel('Password').fill(account.password)
		await activation.getByRole('button', { name: 'Set password' }).click()
		await activation.getByRole('heading', { name: 'Tasks' }).waitFor()
		await context.close()
	} finally {
		await sink.close()
	}
}

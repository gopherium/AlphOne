// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { APIRequestContext, Browser, BrowserContext, Page } from '@playwright/test'

import { baseURL, credentials } from '../env'
import { graph } from '../graph'
import { provisionUser } from '../provision'

/** password is the password every provisioned account signs in with. */
const password = 'correct horse battery'

/**
 * Signs one account in from the login screen in a clean browser context.
 * @param browser - The browser the context opens in.
 * @param email - The address the account signs in with.
 * @returns The signed-in context beside its page.
 */
async function signIn(browser: Browser, email: string): Promise<{ context: BrowserContext; page: Page }> {
	const context = await browser.newContext({ baseURL, storageState: { cookies: [], origins: [] } })
	const page = await context.newPage()
	await page.goto('/')
	await page.getByLabel('Email').fill(email)
	await page.getByLabel('Password').fill(password)
	await page.getByRole('button', { name: 'Log in' }).click()
	await expect(page.getByRole('heading', { level: 1, name: 'Tasks' })).toBeVisible()
	return { context, page }
}

/**
 * Reads whether the caller may change the account holding one address, as the users list answers it.
 * @param request - The request context carrying the caller's credential.
 * @param email - The address of the account.
 * @returns The withinReach flag of that account, undefined when the list leaves it out.
 */
async function withinReachOf(request: APIRequestContext, email: string): Promise<boolean | undefined> {
	const listed = await graph<{ users: { email: string; withinReach: boolean }[] }>(
		request,
		'{ users { email withinReach } }',
	)
	return listed.users.find((user) => user.email === email)?.withinReach
}

test('a member is kept out of the Users screen and finds its API tokens in the menu', async ({ page, browser }) => {
	const stamp = Date.now()
	const email = `member-${stamp}@example.com`
	await provisionUser(page, browser, { email, name: `Maria Perez ${stamp}`, password })

	const member = await signIn(browser, email)
	const menu = member.page.getByRole('navigation', { name: 'Navigation' })
	await expect(menu.getByRole('link', { name: 'Users' })).toHaveCount(0)
	await menu.getByRole('link', { name: 'API tokens' }).click()

	await expect(member.page).toHaveURL(/\/users\/tokens(\?.*)?$/)
	await expect(member.page.getByRole('heading', { level: 1, name: 'API tokens' })).toBeVisible()
	await expect(member.page.getByText('Manage the tokens your programs sign in with.')).toBeVisible()
	await expect(member.page.getByRole('navigation', { name: 'User sections' })).toHaveCount(0)

	await member.page.goto('/users')
	await expect(member.page.getByRole('heading', { level: 1, name: 'Tasks' })).toBeVisible()
	await expect(member.page).toHaveURL(/\/tasks(\?.*)?$/)
	await expect(member.page.getByRole('searchbox', { name: 'Search users…' })).toHaveCount(0)

	await member.page.goto('/users/new')
	await expect(member.page.getByRole('heading', { level: 1, name: 'Tasks' })).toBeVisible()
	await expect(member.page.getByRole('button', { name: 'Send invitation' })).toHaveCount(0)

	await member.context.close()
})

test('a manager is offered the row actions on an account the graph puts within its reach', async ({
	page,
	browser,
	request,
}) => {
	const stamp = Date.now()
	const email = `reach-${stamp}@example.com`
	await provisionUser(page, browser, { email, name: `Maria Perez ${stamp}`, password })

	expect(await withinReachOf(request, email)).toBe(true)
	const member = await signIn(browser, email)
	expect(await withinReachOf(member.context.request, credentials.email)).toBe(false)
	await member.context.close()

	await page.goto(`/users?search=${encodeURIComponent(email)}`)
	await page.getByRole('row').filter({ hasText: email }).getByRole('button', { name: 'Actions' }).click()
	await expect(page.getByRole('menuitem')).toHaveText(['Change role…', 'Disable'])
	await page.keyboard.press('Escape')

	await page.goto(`/users?search=${encodeURIComponent(credentials.email)}`)
	const own = page.getByRole('row').filter({ hasText: credentials.email })
	await expect(own).toBeVisible()
	await expect(own.getByRole('button', { name: 'Actions' })).toHaveCount(0)
})

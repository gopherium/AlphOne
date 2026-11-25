// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

import { credentials } from '../env'
import { graph } from '../graph'
import { provisionUser } from '../provision'

/** password is the password the provisioned account signs in with. */
const password = 'correct horse battery'

test.use({ storageState: { cookies: [], origins: [] } })

/**
 * Signs one account in from the login screen the page shows.
 * @param page - The page showing the login screen.
 * @param email - The address the account signs in with.
 * @param secret - The password the account signs in with.
 */
async function signIn(page: Page, email: string, secret: string): Promise<void> {
	await page.getByLabel('Email').fill(email)
	await page.getByLabel('Password').fill(secret)
	await page.getByRole('button', { name: 'Log in' }).click()
	await expect(page.getByRole('heading', { level: 2, name: 'Tasks' })).toBeVisible()
}

/**
 * Mints one API token for the account the page's credential names.
 * @param page - The page whose cookie names the account.
 * @param name - The name the token takes.
 */
async function mintToken(page: Page, name: string): Promise<void> {
	await graph(
		page.request,
		'mutation($name: String!) { apiTokenCreate(name: $name, scopes: ["tokens:read"]) { token { id } } }',
		{ name },
	)
}

/**
 * Holds every API tokens read the page sends until the release runs.
 * @param page - The page whose reads are held.
 * @returns The promise that settles once a read is held, beside the release.
 */
async function holdTokensRead(page: Page): Promise<{ held: Promise<void>; release: () => void }> {
	let seen: () => void = () => {}
	let release: () => void = () => {}
	const held = new Promise<void>((resolve) => {
		seen = resolve
	})
	const released = new Promise<void>((resolve) => {
		release = resolve
	})
	await page.route('**/api/graphql', async (route) => {
		if (!route.request().postData()?.includes('ApiTokens')) {
			await route.fallback()
			return
		}
		seen()
		await released
		await route.continue()
	})
	return { held, release }
}

test('reads the API tokens afresh for the next account that signs in on the same tab', async ({ page, browser }) => {
	const stamp = Date.now()
	const adminToken = `Admin token ${stamp}`
	const memberToken = `Member token ${stamp}`
	const email = `next-${stamp}@example.com`

	await page.goto('/')
	await signIn(page, credentials.email, credentials.password)
	await provisionUser(page, browser, { email, name: `Maria Perez ${stamp}`, password })
	await mintToken(page, adminToken)

	await page.goto('/users/tokens')
	await expect(page.getByText(adminToken)).toBeVisible()

	const menu = page.getByRole('region', { name: 'Navigation' })
	await menu.getByRole('link', { name: 'Tasks' }).click()
	await expect(page.getByRole('heading', { level: 2, name: 'Tasks' })).toBeVisible()

	await page.getByRole('button', { name: 'Log out' }).click()
	await expect(page.getByLabel('Email')).toBeVisible()

	await signIn(page, email, password)
	await mintToken(page, memberToken)

	const read = await holdTokensRead(page)
	await menu.getByRole('link', { name: 'API tokens' }).click()
	await read.held
	await expect(page.getByRole('heading', { level: 2, name: 'API tokens' })).toBeVisible()
	await expect(page.getByText(adminToken)).toHaveCount(0)

	read.release()
	await expect(page.getByText(memberToken)).toBeVisible()
	await expect(page.getByText(adminToken)).toHaveCount(0)
	await page.unroute('**/api/graphql')
})

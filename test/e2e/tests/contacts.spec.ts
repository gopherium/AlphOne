// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

test('creates a contact, adds and removes an identity, searches, and renames it', async ({
	page,
}) => {
	const stamp = Date.now()
	const name = `Customer ${stamp}`
	const renamed = `Customer ${stamp} Ltd`
	const email = `user${stamp}@example.com`

	await page.goto('/')
	await page.getByRole('link', { name: 'Contacts' }).click()
	await expect(page.getByRole('heading', { name: 'Contacts' })).toBeVisible()

	await page.getByRole('link', { name: 'New contact' }).click()
	await page.getByLabel('Name').fill(name)
	await page.getByRole('button', { name: 'Create contact' }).click()
	await expect(page.getByRole('heading', { name })).toBeVisible()

	await expect(page.getByText('No identities yet.')).toBeVisible()
	await page.getByLabel('Value').fill(email)
	await page.getByLabel('Label').fill('Work')
	await page.getByRole('button', { name: 'Add identity' }).click()
	await expect(page.getByText(`email: ${email} (Work)`)).toBeVisible()

	await page.getByRole('button', { name: `Remove ${email}` }).click()
	await expect(page.getByText('No identities yet.')).toBeVisible()

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('textbox', { name: 'Search contacts' }).fill(String(stamp))
	await page.getByRole('link', { name }).click()
	await expect(page.getByRole('heading', { name })).toBeVisible()

	await page.getByLabel('Name').fill(renamed)
	await page.getByRole('button', { name: 'Save' }).click()
	await expect(page.getByRole('heading', { name: renamed })).toBeVisible()

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('textbox', { name: 'Search contacts' }).fill(String(stamp))
	await expect(page.getByRole('link', { name: renamed })).toBeVisible()
})

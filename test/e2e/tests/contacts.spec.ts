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
	await page.getByLabel('Value', { exact: true }).fill(email)
	await page.getByLabel('Label', { exact: true }).fill('Work')
	await page.getByRole('button', { name: 'Add identity' }).click()
	await expect(page.getByText(`Email: ${email} (Work)`)).toBeVisible()

	await page.getByRole('button', { name: `Remove ${email}` }).click()
	await expect(page.getByText('No identities yet.')).toBeVisible()

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('searchbox', { name: 'Search contacts…' }).fill(String(stamp))
	await page.getByRole('link', { name }).first().click()
	await expect(page.getByRole('heading', { name })).toBeVisible()

	await page.getByLabel('Name', { exact: true }).fill(renamed)
	await page.getByRole('button', { name: 'Save', exact: true }).click()
	await expect(page.getByRole('heading', { name: renamed })).toBeVisible()

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('searchbox', { name: 'Search contacts…' }).fill(String(stamp))
	await expect(page.getByRole('row').filter({ hasText: renamed })).toHaveCount(1)
})

test('renames a contact from its row in a dialog that hands the Name field the focus', async ({ page }) => {
	const stamp = Date.now()
	const name = `Client ${stamp}`
	const renamed = `Client ${stamp} again`

	await page.goto('/contacts/new')
	await page.getByLabel('Name').fill(name)
	await page.getByRole('button', { name: 'Create contact' }).click()
	await expect(page.getByRole('heading', { name })).toBeVisible()

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('searchbox', { name: 'Search contacts…' }).fill(String(stamp))
	const row = page.getByRole('row').filter({ hasText: name })
	await row.getByRole('button', { name: 'Actions' }).click()
	await page.getByRole('menuitem', { name: 'Rename…', exact: true }).click()

	const dialog = page.getByRole('dialog', { name: 'Rename' })
	const field = dialog.getByRole('textbox', { name: 'Name' })
	await expect(field).toBeFocused()
	await field.fill(renamed)
	await dialog.getByRole('button', { name: 'Save', exact: true }).click()

	await expect(dialog).toBeHidden()
	await expect(page.locator('.godmin-toasts').getByText('Name updated.')).toBeVisible()
	await expect(page.getByRole('row').filter({ hasText: renamed })).toHaveCount(1)
})

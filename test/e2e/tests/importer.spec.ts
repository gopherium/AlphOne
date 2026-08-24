// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

/**
 * Maps one column of the import onto a field.
 * @param page - The page showing the import.
 * @param column - The column label.
 * @param field - The field to map it onto.
 */
async function chooseField(page: Page, column: string, field: string) {
	await page.getByRole('combobox', { name: column }).click()
	const listbox = page.getByRole('listbox')
	await listbox.getByRole('option', { name: field, exact: true }).click()
	await expect(listbox).toBeHidden()
}

test('imports a CSV of contacts from the upload through to the contact list', async ({ page }) => {
	const stamp = Date.now()
	const wanted = `Maria Perez ${stamp}`
	const known = `Ana Lopez ${stamp}`
	const knownEmail = `ana.${stamp}@example.com`

	await page.goto('/')
	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('link', { name: 'New contact' }).click()
	await page.getByLabel('Name').fill(known)
	await page.getByRole('button', { name: 'Create contact' }).click()
	await expect(page.getByRole('heading', { name: known })).toBeVisible()
	await page.getByLabel('Value').fill(knownEmail)
	await page.getByRole('button', { name: 'Add identity' }).click()
	await expect(page.getByText(`email: ${knownEmail}`)).toBeVisible()

	await page.getByRole('link', { name: 'Import' }).click()
	await expect(page.getByRole('heading', { name: 'Import' })).toBeVisible()

	const upload = page.waitForResponse(
		(response) =>
			response.url().includes('/api/graphql') &&
			(response.request().headers()['content-type'] ?? '').startsWith('multipart/form-data'),
	)
	await page.getByLabel('Contacts file').setInputFiles({
		name: 'contacts.csv',
		mimeType: 'text/csv',
		buffer: Buffer.from(
			'Full name,Email address\n' +
				`${wanted},maria.${stamp}@example.com\n` +
				`${known},${knownEmail}\n` +
				'No details here,\n',
		),
	})

	const uploaded = await upload
	const body = await uploaded.text()
	expect(uploaded.status(), body).toBe(200)
	expect(body, body).toContain('importUpload')

	await page.getByRole('link', { name: 'contacts.csv' }).click()
	await expect(page.getByRole('heading', { name: 'contacts.csv' })).toBeVisible()
	await expect(page.getByText(wanted)).toBeVisible()

	await chooseField(page, 'Full name', 'Name')
	await chooseField(page, 'Email address', 'Email')
	await page.getByRole('button', { name: 'Save mapping' }).click()

	await page.getByRole('button', { name: 'Commit' }).click()
	const rows = page.getByRole('region', { name: 'Rows' })
	await expect(rows.getByText('Imported', { exact: true }).first()).toBeVisible()
	await expect(rows.getByText('Skipped', { exact: true }).first()).toBeVisible()
	await expect(rows.getByText('Failed', { exact: true }).first()).toBeVisible()

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('textbox', { name: 'Search contacts' }).fill(String(stamp))
	await expect(page.getByRole('link', { name: wanted })).toBeVisible()
	await expect(page.getByRole('link', { name: known })).toHaveCount(1)
})

test('maps a spreadsheet column onto a field an operator defined', async ({ page }) => {
	const stamp = Date.now()
	const wanted = `Maria Perez ${stamp}`
	const field = `joinedOn${stamp}`
	const fieldLabel = `Joined on ${stamp}`

	await page.goto('/')
	await page.getByRole('link', { name: 'Fields' }).click()
	await expect(page.getByRole('heading', { name: 'Fields', level: 1 })).toBeVisible()
	await page.getByLabel('Label').fill(fieldLabel)
	await page.getByLabel('Name').fill(field)
	await page.getByRole('combobox', { name: 'Kind' }).click()
	await page.getByRole('listbox').getByRole('option', { name: 'Date', exact: true }).click()
	await page.getByRole('button', { name: 'Add field' }).click()
	await expect(page.getByRole('region', { name: 'Fields' }).getByText(field)).toBeVisible()

	await page.getByRole('link', { name: 'Import' }).click()
	await page.getByLabel('Contacts file').setInputFiles({
		name: 'joined.csv',
		mimeType: 'text/csv',
		buffer: Buffer.from(
			'Full name,Email address,Joined\n' + `${wanted},maria.${stamp}@example.com,2026-03-01\n`,
		),
	})
	await page.getByRole('link', { name: 'joined.csv' }).click()
	await expect(page.getByRole('heading', { name: 'joined.csv' })).toBeVisible()

	await chooseField(page, 'Full name', 'Name')
	await chooseField(page, 'Email address', 'Email')
	await chooseField(page, 'Joined', fieldLabel)
	await page.getByRole('button', { name: 'Save mapping' }).click()
	await page.getByRole('button', { name: 'Commit' }).click()
	await expect(
		page.getByRole('region', { name: 'Rows' }).getByText('Imported', { exact: true }).first(),
	).toBeVisible()

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('textbox', { name: 'Search contacts' }).fill(String(stamp))
	await page.getByRole('link', { name: wanted }).click()
	await expect(page.getByRole('heading', { name: wanted })).toBeVisible()
	await expect(page.getByLabel(fieldLabel)).toHaveValue('2026-03-01')
})

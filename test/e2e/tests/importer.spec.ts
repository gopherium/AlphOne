// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Locator, Page } from '@playwright/test'

import { graph } from '../graph'

/**
 * Uploads a file through the Upload button of the imports list, then opens it from the toast.
 * @param page - The page showing the imports list.
 * @param file - The file to choose in the dialog.
 */
async function uploadAndOpen(page: Page, file: { name: string; mimeType: string; buffer: Buffer }) {
	const chooser = page.waitForEvent('filechooser')
	await page.getByRole('button', { name: 'Upload' }).click()
	await (await chooser).setFiles(file)
	await expect(page.locator('.godmin-toasts').getByText('File uploaded.')).toBeVisible()
	await page.getByRole('button', { name: 'Open', exact: true }).click()
	await expect(page.getByRole('heading', { name: file.name })).toBeVisible()
}

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

/**
 * Measures how far a table runs past the box that scrolls it sideways.
 * @param table - The table to measure.
 * @returns The hidden width in pixels, zero when the table fits.
 */
async function hiddenWidth(table: Locator): Promise<number> {
	return table.evaluate((element) => {
		let box = element.parentElement
		while (box !== null && !['auto', 'scroll'].includes(getComputedStyle(box).overflowX)) {
			box = box.parentElement
		}
		return box === null ? 0 : box.scrollWidth - box.clientWidth
	})
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
	await expect(page.getByText(`Email: ${knownEmail}`)).toBeVisible()

	await page.getByRole('link', { name: 'Import' }).click()
	await expect(page.getByRole('heading', { name: 'Import', level: 1 })).toBeVisible()

	const upload = page.waitForResponse(
		(response) =>
			response.url().includes('/api/graphql') &&
			(response.request().headers()['content-type'] ?? '').startsWith('multipart/form-data'),
	)
	await uploadAndOpen(page, {
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

	await expect(page.getByText(wanted)).toBeVisible()

	await chooseField(page, 'Full name', 'Name')
	await chooseField(page, 'Email address', 'Email')
	await page.getByRole('button', { name: 'Save mapping' }).click()

	await page.getByRole('button', { name: 'Commit' }).click()
	const rows = page.getByRole('region', { name: 'Rows' })
	await expect(rows.getByText('Imported', { exact: true }).first()).toBeVisible()
	await expect(rows.getByText('Skipped', { exact: true }).first()).toBeVisible()
	await expect(rows.getByText('Failed', { exact: true }).first()).toBeVisible()
	await expect(rows.getByText(`${known} already holds an address in this row.`)).toBeVisible()
	await expect(rows.getByText('The row has no name or no address.')).toBeVisible()

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('searchbox', { name: 'Search contacts…' }).fill(String(stamp))
	await expect(page.getByRole('row').filter({ hasText: wanted })).toHaveCount(1)
	await expect(page.getByRole('row').filter({ hasText: known })).toHaveCount(1)
})

test('wraps a long row reason so the rows table fits a laptop screen', async ({ page, request }) => {
	const stamp = Date.now()
	const owner = `Maria Perez de la Fuente y Rodriguez ${stamp}`
	const ownerEmail = `${stamp}@example.com`
	await graph(
		request,
		'mutation($name: String!, $identities: [ContactIdentityInput!]) {' +
			' createContact(name: $name, identities: $identities) { id } }',
		{ name: owner, identities: [{ channel: 'email', identifier: ownerEmail }] },
	)

	await page.setViewportSize({ width: 1280, height: 900 })
	await page.goto('/import')
	await uploadAndOpen(page, {
		name: `owned-${stamp}.csv`,
		mimeType: 'text/csv',
		buffer: Buffer.from('Full name,Email address\n' + `Ana ${stamp},${ownerEmail}\n`),
	})
	await chooseField(page, 'Full name', 'Name')
	await chooseField(page, 'Email address', 'Email')
	await page.getByRole('button', { name: 'Save mapping' }).click()
	await page.getByRole('button', { name: 'Commit' }).click()

	const rows = page.getByRole('region', { name: 'Rows' })
	await expect(rows.getByText(`${owner} already holds an address in this row.`)).toBeVisible()
	expect(await hiddenWidth(rows.getByRole('table'))).toBe(0)
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
	await page.getByRole('combobox', { name: 'Kind' }).click()
	await page.getByRole('listbox').getByRole('option', { name: 'Date', exact: true }).click()
	await page.getByRole('button', { name: 'Add field' }).click()
	await expect(page.getByRole('region', { name: 'Fields' }).getByText(field)).toBeVisible()

	await page.getByRole('link', { name: 'Import' }).click()
	await uploadAndOpen(page, {
		name: 'joined.csv',
		mimeType: 'text/csv',
		buffer: Buffer.from(
			'Full name,Email address,Joined\n' + `${wanted},maria.${stamp}@example.com,2026-03-01\n`,
		),
	})

	await chooseField(page, 'Full name', 'Name')
	await chooseField(page, 'Email address', 'Email')
	await chooseField(page, 'Joined', fieldLabel)
	await page.getByRole('button', { name: 'Save mapping' }).click()
	await page.getByRole('button', { name: 'Commit' }).click()
	await expect(
		page.getByRole('region', { name: 'Rows' }).getByText('Imported', { exact: true }).first(),
	).toBeVisible()

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('searchbox', { name: 'Search contacts…' }).fill(String(stamp))
	await page.getByRole('link', { name: wanted }).first().click()
	await expect(page.getByRole('heading', { name: wanted })).toBeVisible()
	await expect(page.getByLabel(fieldLabel)).toHaveValue('2026-03-01')
})

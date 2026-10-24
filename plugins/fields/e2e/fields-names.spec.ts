// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

/**
 * Adds one Text field from the Fields screen and waits for its row.
 * @param page - The page showing the Fields screen.
 * @param label - The field's label.
 */
async function addTextField(page: Page, label: string) {
	await page.getByLabel('Label', { exact: true }).fill(label)
	await page.getByRole('combobox', { name: 'Kind' }).click()
	const listbox = page.getByRole('listbox')
	await listbox.getByRole('option', { name: 'Text', exact: true }).click()
	await expect(listbox).toBeHidden()
	await page.getByRole('button', { name: 'Add field' }).click()
	await expect(fieldRow(page, label)).toBeVisible()
}

/**
 * Returns the row of the Fields list that holds a label.
 * @param page - The page showing the Fields screen.
 * @param label - The field's label.
 * @returns The row.
 */
function fieldRow(page: Page, label: string) {
	return page.getByRole('region', { name: 'Fields' }).getByRole('row').filter({ hasText: label })
}

test('names each new field from its label and numbers a name already taken', async ({ page }) => {
	const stamp = Date.now()

	await page.goto('/')
	await page.getByRole('link', { name: 'Fields' }).click()
	await expect(page.getByRole('heading', { name: 'Fields', level: 1 })).toBeVisible()
	await addTextField(page, `Visit ${stamp} notes`)
	await addTextField(page, `Visit ${stamp}: notes`)

	await expect(fieldRow(page, `Visit ${stamp} notes`)).toContainText(`visit${stamp}Notes`)
	await expect(fieldRow(page, `Visit ${stamp}: notes`)).toContainText(`visit${stamp}Notes2`)
})

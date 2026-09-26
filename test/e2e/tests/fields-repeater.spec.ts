// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Locator, Page } from '@playwright/test'

/**
 * Chooses one kind from a kind menu and waits for the menu to close.
 * @param page - The page showing the menu.
 * @param menu - The kind combobox.
 * @param kind - The kind option to choose.
 */
async function chooseKind(page: Page, menu: Locator, kind: string) {
	await menu.click()
	const listbox = page.getByRole('listbox')
	await listbox.getByRole('option', { name: kind, exact: true }).click()
	await expect(listbox).toBeHidden()
}

/**
 * Adds one sub field to the repeater the add form holds.
 * @param page - The page showing the Fields screen.
 * @param at - The place the sub field takes, counting from 1.
 * @param label - The sub field label.
 * @param kind - The kind option to choose.
 */
async function addSubField(page: Page, at: number, label: string, kind: string) {
	await page.getByRole('button', { name: 'Add sub field' }).click()
	const row = page.getByRole('group', { name: `Sub field ${at}`, exact: true })
	await row.getByLabel('Label', { exact: true }).fill(label)
	await chooseKind(page, row.getByRole('combobox', { name: 'Kind' }), kind)
}

test('defines a repeater and keeps a contact history in order', async ({ page }) => {
	const stamp = Date.now()
	const label = `History ${stamp}`
	const contact = `Customer ${stamp}`

	await page.goto('/')
	await page.getByRole('link', { name: 'Fields' }).click()
	await expect(page.getByRole('heading', { name: 'Fields', level: 1 })).toBeVisible()
	await page.getByLabel('Label', { exact: true }).fill(label)
	await page.getByLabel('Name', { exact: true }).fill(`history${stamp}`)
	await chooseKind(page, page.getByRole('combobox', { name: 'Kind' }), 'Repeater')
	await addSubField(page, 1, 'Date', 'Date')
	await addSubField(page, 2, 'Comment', 'Long text')
	await page.getByRole('button', { name: 'Add field' }).click()
	const defined = page.getByRole('region', { name: 'Fields' }).getByRole('row').filter({ hasText: label })
	await expect(defined).toContainText('Repeater')
	await expect(defined).toContainText('Date, Comment')

	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('link', { name: 'New contact' }).click()
	await page.getByLabel('Name', { exact: true }).fill(contact)
	await page.getByRole('button', { name: 'Create contact' }).click()
	await expect(page.getByRole('heading', { name: contact })).toBeVisible()

	const history = page.getByRole('group', { name: label, exact: true })
	await expect(history.getByText('No entries yet.')).toBeVisible()
	await page.getByRole('button', { name: `Add an entry to ${label}` }).click()
	const first = page.getByRole('group', { name: `${label} 1`, exact: true })
	await first.getByLabel('Date', { exact: true }).fill('2026-09-01')
	await first.getByLabel('Comment', { exact: true }).fill('First call about the yearly plan.\nAsked for a quote.')
	await page.getByRole('button', { name: `Add an entry to ${label}` }).click()
	const second = page.getByRole('group', { name: `${label} 2`, exact: true })
	await second.getByLabel('Date', { exact: true }).fill('2026-09-10')
	await second.getByLabel('Comment', { exact: true }).fill('Sent the offer.')
	await second.getByRole('button', { name: 'Move entry up' }).click()
	await page.getByRole('button', { name: 'Save fields' }).click()

	await page.reload()
	const kept = page.getByRole('group', { name: `${label} 1`, exact: true })
	await expect(kept.getByLabel('Date', { exact: true })).toHaveValue('2026-09-10')
	await expect(kept.getByLabel('Comment', { exact: true })).toHaveValue('Sent the offer.')
	const moved = page.getByRole('group', { name: `${label} 2`, exact: true })
	await expect(moved.getByLabel('Date', { exact: true })).toHaveValue('2026-09-01')
	const comment = moved.getByRole('textbox', { name: 'Comment', exact: true })
	await expect(comment).toHaveValue('First call about the yearly plan.\nAsked for a quote.')
	await expect(comment).toHaveJSProperty('tagName', 'TEXTAREA')
})

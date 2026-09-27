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

test('defines a repeater and keeps a contact history one entry at a time', async ({ page }) => {
	const stamp = Date.now()
	const label = `History ${stamp}`
	const contact = `Customer ${stamp}`
	const sent = operationsSent(page)

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
	const form = history.getByRole('form', { name: `Add an entry to ${label}` })
	await expect(form.getByRole('textbox', { name: 'Comment', exact: true })).toHaveJSProperty('tagName', 'TEXTAREA')
	await addEntry(page, form, '2026-09-01', 'First call about the yearly plan.\nAsked for a quote.')
	await addEntry(page, form, '2026-09-10', 'Sent the offer.')
	await addEntry(page, form, '2026-09-18', 'Follow-up call.')
	const entries = history.getByRole('listitem')
	await expect(entries.first()).toHaveAccessibleName('Sep 18, 2026, Follow-up call.')

	const removed = operationAnswer(page, 'DeleteContactFieldEntry')
	await history.getByRole('button', { name: 'Remove entry: Sep 10, 2026, Sent the offer.' }).click()
	await history.getByRole('button', { name: 'Remove', exact: true }).click()
	await removed

	const firstCall = 'Sep 1, 2026, First call about the yearly plan.'
	await history.getByRole('button', { name: `Edit entry: ${firstCall}` }).click()
	const editor = history.getByRole('form', { name: `Edit entry: ${firstCall}` })
	const edited = editor.getByRole('textbox', { name: 'Comment', exact: true })
	await expect(edited).toHaveJSProperty('tagName', 'TEXTAREA')
	await edited.fill('First call about the yearly plan.\nAsked for a second quote.')
	const saved = operationAnswer(page, 'UpdateContactFieldEntry')
	await editor.getByRole('button', { name: 'Save entry' }).click()
	await saved

	await page.reload()
	await expect(entries).toHaveCount(2)
	await expect(entries.nth(0)).toHaveAccessibleName('Sep 18, 2026, Follow-up call.')
	await expect(entries.nth(1)).toHaveAccessibleName(firstCall)
	const body = entries.nth(1).locator('p.godmin-log-list__body')
	expect(await body.evaluate((node) => (node as HTMLElement).innerText)).toBe(
		'First call about the yearly plan.\nAsked for a second quote.',
	)
	const days = history.locator('time')
	await expect(days).toHaveCount(2)
	expect(await days.evaluateAll((nodes) => nodes.map((node) => node.getAttribute('datetime')))).toEqual([
		'2026-09-18',
		'2026-09-01',
	])
	expect(sent).toEqual(
		expect.arrayContaining(['AddContactFieldEntry', 'DeleteContactFieldEntry', 'UpdateContactFieldEntry']),
	)
	expect(sent).not.toContain('WriteContactFields')
})

/**
 * Collects the name of every operation the page sends to the graph.
 * @param page - The page sending the operations.
 * @returns The operation names, growing as the page sends them.
 */
function operationsSent(page: Page): string[] {
	const names: string[] = []
	page.on('request', (request) => {
		if (request.method() === 'POST' && request.url().includes('/api/graphql')) {
			const body = request.postDataJSON() as { operationName?: string } | null
			names.push(body?.operationName ?? '')
		}
	})
	return names
}

/**
 * Waits for the graph to answer the named operation.
 * @param page - The page sending the operation.
 * @param operation - The operation name.
 * @returns The answer, once it arrives.
 */
function operationAnswer(page: Page, operation: string) {
	return page.waitForResponse(
		(response) =>
			response.url().includes('/api/graphql') && (response.request().postData() ?? '').includes(operation),
	)
}

/**
 * Adds one entry through a repeater's add form and waits until the form is ready for the next one.
 * @param page - The page showing the contact.
 * @param form - The add form.
 * @param date - The entry's date.
 * @param comment - The entry's comment.
 */
async function addEntry(page: Page, form: Locator, date: string, comment: string) {
	const text = form.getByLabel('Comment', { exact: true })
	await form.getByLabel('Date', { exact: true }).fill(date)
	await text.fill(comment)
	const stored = operationAnswer(page, 'AddContactFieldEntry')
	await form.getByRole('button', { name: /^Add an entry to / }).click()
	await stored
	await expect(text).toHaveValue('')
}

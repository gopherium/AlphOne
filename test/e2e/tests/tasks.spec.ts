// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

const doneGroup = /^Done \(\d+\)$/

/**
 * Adds a task to the day on screen through the quick add field.
 * @param page - The page showing a day of tasks.
 * @param title - The title of the task to add.
 */
async function quickAdd(page: Page, title: string) {
	await page.getByRole('textbox', { name: 'New task' }).fill(title)
	await page.getByRole('button', { name: 'Add task' }).click()
	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
}

test('adds, completes, and reopens a task', async ({ page }) => {
	const title = `Call the supplier ${Date.now()}`

	await page.goto('/')
	await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()
	await quickAdd(page, title)

	const openTasks = page.getByRole('list', { name: 'Open tasks' })
	await expect(openTasks.getByRole('link', { name: title })).toBeVisible()

	await page.getByRole('listitem', { name: title }).getByRole('button', { name: 'Complete' }).click()

	await expect(openTasks.getByRole('link', { name: title })).toBeHidden()
	await page.getByRole('button', { name: doneGroup }).click()
	const doneTasks = page.getByRole('list', { name: 'Done tasks' })
	await expect(doneTasks.getByRole('link', { name: title })).toBeVisible()

	await page.getByRole('listitem', { name: title }).getByRole('button', { name: 'Reopen' }).click()

	await expect(openTasks.getByRole('link', { name: title })).toBeVisible()
})

test('pushes a task to the next day', async ({ page }) => {
	const title = `Send the quote ${Date.now()}`

	await page.goto('/')
	await quickAdd(page, title)

	await page
		.getByRole('listitem', { name: title })
		.getByRole('button', { name: 'Push to tomorrow' })
		.click()

	await expect(page.getByRole('listitem', { name: title })).toBeHidden()

	await page.getByRole('link', { name: 'Next day' }).click()

	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
})

test('carries work left over from an earlier day into today', async ({ page }) => {
	const title = `Chase the invoice ${Date.now()}`

	await page.goto('/')
	await page.getByRole('link', { name: 'Previous day' }).click()
	await quickAdd(page, title)

	await page.getByRole('link', { name: 'Today' }).click()

	const overdue = page.getByRole('list', { name: 'Overdue tasks' })
	await expect(overdue.getByRole('link', { name: title })).toBeVisible()

	await page
		.getByRole('listitem', { name: title })
		.getByRole('button', { name: 'Push to tomorrow' })
		.click()

	await expect(page.getByRole('listitem', { name: title })).toBeHidden()

	await page.getByRole('link', { name: 'Next day' }).click()

	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
})

test('opens a task from the day list', async ({ page }) => {
	const title = `Approve the pricing ${Date.now()}`

	await page.goto('/')
	await quickAdd(page, title)

	await page.getByRole('link', { name: title }).click()

	await expect(page.getByRole('heading', { name: title })).toBeVisible()
	await expect(page.getByLabel('Title')).toHaveValue(title)

	await page.getByLabel('Title').fill(`${title} today`)
	await page.getByRole('button', { name: 'Save' }).click()

	await expect(page.getByRole('heading', { name: `${title} today` })).toBeVisible()
})

test('adds a task from a contact and links it back', async ({ page }) => {
	const stamp = Date.now()
	const contact = `Customer ${stamp}`
	const title = `Call ${contact}`

	await page.goto('/')
	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('link', { name: 'New contact' }).click()
	await page.getByLabel('Name').fill(contact)
	await page.getByRole('button', { name: 'Create contact' }).click()
	await expect(page.getByRole('heading', { name: contact })).toBeVisible()

	await page.getByRole('textbox', { name: 'New task for this contact' }).fill(title)
	await page.getByRole('button', { name: 'Add task' }).click()

	const contactTasks = page.getByRole('list', { name: 'Contact tasks' })
	await expect(contactTasks.getByRole('link', { name: title })).toBeVisible()

	await contactTasks.getByRole('link', { name: title }).click()

	await expect(page.getByRole('heading', { name: title })).toBeVisible()
	await expect(page.getByRole('link', { name: contact })).toBeVisible()
})

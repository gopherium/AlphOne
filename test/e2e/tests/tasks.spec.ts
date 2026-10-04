// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { APIRequestContext, Page } from '@playwright/test'

import { createTask, graph } from '../graph'

const doneGroup = /^Done \(\d+\)$/

/** levelDay is the day the row geometry checks list their tasks on, kept clear of the days other tests fill. */
const levelDay = '2125-03-10'

/** LinkedPair names a task linked to a contact and a task with no contact, titled alike. */
type LinkedPair = { linked: string; unlinked: string }

/**
 * Adds a task to the day on screen through the quick add field.
 * @param page - The page showing a day of tasks.
 * @param title - The title of the task to add.
 */
async function quickAdd(page: Page, title: string) {
	await page.getByRole('textbox', { name: 'Task title' }).fill(title)
	await page.getByRole('button', { name: 'Add task' }).click()
	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
}

/**
 * Creates two tasks on the level check day with titles of the same width, the first linked to a new contact.
 * @param request - The request context carrying the credential.
 * @param title - The words both titles start with.
 * @returns The titles of the linked and the unlinked task.
 */
async function seedLinkedPair(request: APIRequestContext, title: string): Promise<LinkedPair> {
	const stamp = Date.now()
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: `Maria Perez ${stamp}` },
	)
	const pair = { linked: `${title} ${stamp}1`, unlinked: `${title} ${stamp}2` }
	await createTask(request, { title: pair.linked, dueOn: levelDay, contactId: created.createContact.id })
	await createTask(request, { title: pair.unlinked, dueOn: levelDay })
	return pair
}

/**
 * Measures a task row inside its borders, its title top from that inner top, and the lines its title wraps to.
 * @param page - The page showing the row.
 * @param title - The title of the task.
 * @returns The inner row height, the title top from the inner row top, and the title line count.
 */
async function rowGeometry(page: Page, title: string) {
	const row = page.getByRole('listitem', { name: title, exact: true })
	await expect(row).toBeVisible()
	const inner = await row.evaluate((node) => {
		const box = node.getBoundingClientRect()
		const style = getComputedStyle(node)
		const top = box.top + Number.parseFloat(style.borderTopWidth)
		return { top, height: box.bottom - Number.parseFloat(style.borderBottomWidth) - top }
	})
	const lines = await row
		.getByRole('link', { name: title, exact: true })
		.evaluate((link) => [...link.getClientRects()].map((line) => line.top))
	return { height: inner.height, titleTop: lines[0] - inner.top, lines: lines.length }
}

/**
 * Checks that the linked row matches the unlinked row in height and title top, both titles on the given lines.
 * @param page - The page showing both rows.
 * @param pair - The titles of the linked and the unlinked task.
 * @param lines - How many lines each title wraps to.
 */
async function expectLevelRows(page: Page, pair: LinkedPair, lines: number) {
	await expect(
		page.getByRole('listitem', { name: pair.linked, exact: true }).getByRole('link', { name: /^Open / }),
	).toBeVisible()
	const linked = await rowGeometry(page, pair.linked)
	const unlinked = await rowGeometry(page, pair.unlinked)

	expect(linked.lines, 'the linked title wraps as asked').toBe(lines)
	expect(unlinked.lines, 'the unlinked title wraps as asked').toBe(lines)
	expect(linked.height, 'the person icon grows the row').toBeCloseTo(unlinked.height, 0)
	expect(linked.titleTop, 'the person icon moves the title').toBeCloseTo(unlinked.titleTop, 0)
}

test('adds, completes, and reopens a task', async ({ page }) => {
	const title = `Call the supplier ${Date.now()}`

	await page.goto('/')
	await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()
	await quickAdd(page, title)

	const openTasks = page.getByRole('list', { name: 'Open tasks' })
	await expect(openTasks.getByRole('link', { name: title })).toBeVisible()

	await page.getByRole('listitem', { name: title }).getByRole('checkbox', { name: 'Complete' }).click()

	await expect(openTasks.getByRole('link', { name: title })).toBeHidden()
	await page.getByRole('button', { name: doneGroup }).click()
	const doneTasks = page.getByRole('list', { name: 'Done tasks' })
	await expect(doneTasks.getByRole('link', { name: title })).toBeVisible()

	await page.getByRole('listitem', { name: title }).getByRole('checkbox', { name: 'Reopen' }).click()

	await expect(openTasks.getByRole('link', { name: title })).toBeVisible()
})

test('pushes a task to the next day', async ({ page }) => {
	const title = `Send the quote ${Date.now()}`

	await page.goto('/')
	await quickAdd(page, title)

	await page
		.getByRole('listitem', { name: title })
		.getByRole('button', { name: 'Postpone' })
		.click()

	await expect(page.getByRole('listitem', { name: title })).toBeHidden()

	await page.getByRole('button', { name: 'Next day' }).click()

	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
})

test('carries work left over from an earlier day into today', async ({ page }) => {
	const title = `Chase the invoice ${Date.now()}`

	await page.goto('/')
	await page.getByRole('button', { name: 'Previous day' }).click()
	await quickAdd(page, title)

	await page.getByRole('button', { name: 'Today' }).click()

	const overdue = page.getByRole('list', { name: 'Overdue tasks' })
	await expect(overdue.getByRole('link', { name: title })).toBeVisible()

	await page
		.getByRole('listitem', { name: title })
		.getByRole('button', { name: 'Postpone' })
		.click()

	await expect(page.getByRole('listitem', { name: title })).toBeHidden()

	await page.getByRole('button', { name: 'Next day' }).click()

	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
})

test('adds a task with a due date and a priority from the tasks screen', async ({ page }) => {
	const title = `Order more boxes ${Date.now()}`

	await page.goto('/')
	await expect(page.getByRole('heading', { name: 'Tasks', exact: true })).toBeVisible()
	const browserDay = await page.evaluate(() => {
		const at = new Date()
		const today = at.toLocaleDateString('en-CA')
		at.setDate(at.getDate() + 1)
		return { today, tomorrow: at.toLocaleDateString('en-CA') }
	})

	await page.getByRole('textbox', { name: 'Task title', exact: true }).fill(title)
	await page.getByLabel('Due date', { exact: true }).fill(browserDay.tomorrow)
	await page.getByLabel('Priority', { exact: true }).click()
	await page.getByRole('option', { name: 'High', exact: true }).click()
	await page.getByRole('button', { name: 'Add task', exact: true }).click()

	const toasts = page.locator('.godmin-toasts')
	const added = `Task added for ${browserDay.tomorrow.split('-').reverse().join('/')}.`
	await expect(toasts.getByText(added, { exact: true })).toBeVisible()
	await expect(page.getByRole('textbox', { name: 'Task title', exact: true })).toHaveValue('')
	await expect(page.getByRole('textbox', { name: 'Task title', exact: true })).toBeFocused()
	await expect(page.getByLabel('Due date', { exact: true })).toHaveValue(browserDay.today)
	await expect(page.getByLabel('Priority', { exact: true })).toHaveText('Normal')
	await expect(page.getByRole('listitem', { name: title, exact: true })).toHaveCount(0)

	await toasts.getByRole('button', { name: 'View', exact: true }).click()

	const openTasks = page.getByRole('list', { name: 'Open tasks', exact: true })
	const row = openTasks.getByRole('listitem', { name: title, exact: true })
	await expect(row).toBeVisible()
	await expect(row.getByText('High', { exact: true })).toBeVisible()
})

test('opens the contact of a task from the person icon on its row', async ({ page, request }) => {
	const stamp = Date.now()
	const contact = `Maria Perez ${stamp}`
	const title = `Call ${contact} back`
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: contact },
	)
	const browserToday = await page.evaluate(() => new Date().toLocaleDateString('en-CA'))
	await createTask(request, {
		title,
		dueOn: browserToday,
		contactId: created.createContact.id,
	})

	await page.goto('/tasks')
	const row = page.getByRole('listitem', { name: title, exact: true })
	await row.getByRole('link', { name: `Open ${contact}`, exact: true }).click()

	await expect(page).toHaveURL(new RegExp(`/contacts/${created.createContact.id}$`))
	await expect(page.getByRole('heading', { level: 1, name: contact, exact: true })).toBeVisible()
})

test('keeps a row with the person icon as tall as a row without, titles level, on a desktop', async ({
	page,
	request,
}) => {
	const titles = await seedLinkedPair(request, 'Confirm the delivery window')

	await page.setViewportSize({ width: 1280, height: 800 })
	await page.goto(`/tasks?date=${levelDay}`)

	await expectLevelRows(page, titles, 1)
})

test('keeps a wrapped row with the person icon as tall as a wrapped row without, titles level, on a phone', async ({
	page,
	request,
}) => {
	const titles = await seedLinkedPair(request, 'Confirm the delivery window with the warehouse team')

	await page.setViewportSize({ width: 390, height: 844 })
	await page.goto(`/tasks?date=${levelDay}`)

	await expectLevelRows(page, titles, 2)
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

	await page.getByRole('textbox', { name: 'Task title' }).fill(title)
	await page.getByRole('button', { name: 'Add task' }).click()

	const contactTasks = page.getByRole('list', { name: 'Contact tasks' })
	await expect(contactTasks.getByRole('link', { name: title })).toBeVisible()

	await contactTasks.getByRole('link', { name: title }).click()

	await expect(page.getByRole('heading', { name: title })).toBeVisible()
	await expect(page.getByRole('link', { name: contact })).toBeVisible()
})

test('adds a task from a contact on a chosen day', async ({ page }) => {
	const stamp = Date.now()
	const contact = `Customer ${stamp}`
	const title = `Follow up with ${contact}`
	const chosen = new Date()
	chosen.setDate(chosen.getDate() + 3)
	const due = chosen.toLocaleDateString('en-CA')
	const label = `Due ${due.split('-').reverse().join('/')}`

	await page.goto('/')
	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('link', { name: 'New contact' }).click()
	await page.getByLabel('Name').fill(contact)
	await page.getByRole('button', { name: 'Create contact' }).click()
	await expect(page.getByRole('heading', { name: contact })).toBeVisible()

	await page.getByRole('textbox', { name: 'Task title' }).fill(title)
	await page.getByLabel('Due date', { exact: true }).fill(due)
	await page.getByRole('button', { name: 'Add task' }).click()

	const row = page.getByRole('list', { name: 'Contact tasks' }).getByRole('listitem', { name: title })
	await expect(row).toContainText(label)
	const browserToday = await page.evaluate(() => new Date().toLocaleDateString('en-CA'))
	await expect(page.getByLabel('Due date', { exact: true })).toHaveValue(browserToday)

	await row.getByRole('link', { name: title }).click()

	await expect(page.getByRole('heading', { name: title })).toBeVisible()
	await expect(page.getByLabel('Due date', { exact: true })).toHaveValue(due)
})

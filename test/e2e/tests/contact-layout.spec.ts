// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { APIRequestContext, Page } from '@playwright/test'

import { createTask, graph } from '../graph'

/** Box is the part of a bounding box the layout checks read. */
type Box = { x: number; y: number; width: number; height: number }

/**
 * Creates a contact holding one open task, with a text field defined, and returns the contact id.
 * @param request - The request context carrying the credential.
 * @param stamp - The number that keeps this run's names apart.
 * @returns The id of the created contact.
 */
async function seedContact(request: APIRequestContext, stamp: number): Promise<string> {
	await graph(
		request,
		'mutation($name: String!, $label: String!) { defineField(name: $name, label: $label, kind: TEXT) { id } }',
		{ name: `region${stamp}`, label: `Region ${stamp}` },
	)
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: `Customer ${stamp}` },
	)
	await createTask(request, {
		title: `Approve the pricing before the renewal ${stamp}`,
		dueOn: new Date().toISOString().slice(0, 10),
		contactId: created.createContact.id,
	})
	return created.createContact.id
}

/**
 * Returns the bounding box of a locator, failing when it renders none.
 * @param page - The page showing the contact.
 * @param found - What to measure.
 * @returns The box.
 */
async function boxOf(page: Page, found: ReturnType<Page['locator']>): Promise<Box> {
	await expect(found).toBeVisible()
	const box = await found.boundingBox()
	expect(box, 'the element renders no box').not.toBeNull()
	return box as Box
}

/**
 * Opens a seeded contact at a viewport size and returns its task list and Fields heading boxes.
 * @param page - The page to drive.
 * @param request - The request context carrying the credential.
 * @param size - The viewport to open the contact at.
 * @returns The boxes of the task list and the Fields heading.
 */
async function openContact(page: Page, request: APIRequestContext, size: { width: number; height: number }) {
	const stamp = Date.now()
	const id = await seedContact(request, stamp)
	await page.setViewportSize(size)
	await page.goto(`/contacts/${id}`)
	const list = await boxOf(page, page.getByRole('list', { name: 'Contact tasks' }))
	const fields = await boxOf(page, page.getByRole('heading', { name: 'Fields', exact: true }))
	return { stamp, list, fields }
}

test('sets the fields beside the tasks on a desktop', async ({ page, request }) => {
	const { list, fields } = await openContact(page, request, { width: 1280, height: 720 })

	expect(fields.x).toBeGreaterThanOrEqual(list.x + list.width)
	expect(fields.y).toBeLessThan(list.y + list.height)
})

test('keeps the new task form and a task title inside their column on a tablet', async ({ page, request }) => {
	const { stamp, fields } = await openContact(page, request, { width: 800, height: 1000 })
	const main = await boxOf(page, page.locator('.godmin-page__main'))
	const form = await boxOf(page, page.locator('.alphone-tasks__add--contact'))

	expect(fields.x, 'the tablet shows two columns').toBeGreaterThan(main.x + main.width)
	expect(form.x + form.width).toBeLessThanOrEqual(main.x + main.width + 0.5)
	const share = await page.evaluate((name) => {
		const row = [...document.querySelectorAll('.alphone-tasks__row')].find(
			(candidate) => candidate.getAttribute('aria-label') === name,
		)
		const title = row?.querySelector('.alphone-tasks__title')
		if (!row || !title) {
			throw new Error('the contact rendered no task row')
		}
		return title.getBoundingClientRect().width / row.getBoundingClientRect().width
	}, `Approve the pricing before the renewal ${stamp}`)
	expect(share).toBeGreaterThan(0.5)
})

test('moves the fields under the tasks on a phone', async ({ page, request }) => {
	const { list, fields } = await openContact(page, request, { width: 390, height: 844 })

	expect(fields.y).toBeGreaterThanOrEqual(list.y + list.height)
})

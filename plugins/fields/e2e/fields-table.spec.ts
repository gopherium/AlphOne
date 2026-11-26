// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { APIRequestContext, Locator, Page } from '@playwright/test'

import { graph } from '../../../test/e2e/graph'

/** Box is the top and the height of what an element draws. */
interface Box {
	y: number
	height: number
}

/** SeededFields are the labels of a Text field and of a repeater one test defined. */
interface SeededFields {
	text: string
	repeater: string
}

/** DEFINE_TEXT defines one Text field. */
const DEFINE_TEXT =
	'mutation($name: String!, $label: String!) { defineField(name: $name, label: $label, kind: TEXT) { id } }'

/** DEFINE_REPEATER defines one repeater with its sub fields. */
const DEFINE_REPEATER = `mutation($name: String!, $label: String!, $subFields: [FieldSubFieldInput!]) {
	defineField(name: $name, label: $label, kind: REPEATER, subFields: $subFields) { id }
}`

/**
 * Defines one Text field and one repeater with two sub fields through the graph.
 * @param request - The request context carrying the credential.
 * @returns The label of the Text field and the label of the repeater.
 */
async function seedFields(request: APIRequestContext): Promise<SeededFields> {
	const stamp = Date.now()
	const text = `Nickname ${stamp}`
	const repeater = `Visits ${stamp}`
	await graph(request, DEFINE_TEXT, { name: `nickname${stamp}`, label: text })
	await graph(request, DEFINE_REPEATER, {
		name: `visits${stamp}`,
		label: repeater,
		subFields: [
			{ name: 'date', label: 'Date', kind: 'DATE' },
			{ name: 'comment', label: 'Comment', kind: 'LONGTEXT' },
		],
	})
	return { text, repeater }
}

/**
 * Opens the Fields screen and waits for its title and its fonts.
 * @param page - The page to open it in.
 */
async function openFields(page: Page) {
	await page.goto('/fields')
	await expect(page.getByRole('heading', { name: 'Fields', level: 2 })).toBeVisible()
	await page.evaluate(() => document.fonts.ready)
}

/**
 * Returns the region the field list renders in.
 * @param page - The page showing the Fields screen.
 * @returns The region.
 */
function fieldRegion(page: Page) {
	return page.getByRole('region', { name: 'Fields' })
}

/**
 * Returns the row of the field list that holds a label.
 * @param page - The page showing the Fields screen.
 * @param label - The field's label.
 * @returns The row.
 */
function fieldRow(page: Page, label: string) {
	return fieldRegion(page).getByRole('row').filter({ hasText: label })
}

/**
 * Returns the box the first line of a row's label text draws, not the cell around it.
 * @param row - The field's row.
 * @returns The box of the label's first line.
 */
function labelText(row: Locator): Promise<Box> {
	return row
		.getByRole('cell')
		.first()
		.evaluate((cell) => {
			const range = document.createRange()
			range.selectNodeContents(document.createTreeWalker(cell, NodeFilter.SHOW_TEXT).nextNode() as Text)
			const { top, height } = range.getClientRects()[0]
			return { y: top, height }
		})
}

/**
 * Returns the trash button of a field's row.
 * @param row - The field's row.
 * @param label - The field's label.
 * @returns The trash button.
 */
function trashOf(row: Locator, label: string) {
	return row.getByRole('button', { name: `Archive ${label}`, exact: true })
}

/**
 * Returns the box an element draws, failing the test when it draws none.
 * @param target - The element.
 * @returns Its box.
 */
async function boxOf(target: Locator): Promise<Box> {
	const box = await target.boundingBox()
	expect(box, 'the element draws a box').not.toBeNull()
	return box as Box
}

/**
 * Returns the vertical middle of a box.
 * @param box - The box.
 * @returns Its middle.
 */
function middle(box: Box) {
	return box.y + box.height / 2
}

/**
 * Checks that the text of a field's label sits level with the field's trash, their middles within 1px.
 * @param page - The page showing the Fields screen.
 * @param label - The field's label.
 */
async function expectLevel(page: Page, label: string) {
	const row = fieldRow(page, label)
	await expect(row).toBeVisible()
	const text = await labelText(row)
	const trash = await boxOf(trashOf(row, label))
	const apart = Math.abs(middle(text) - middle(trash))
	expect(apart, `the label of ${label} is level with its trash`).toBeLessThanOrEqual(1)
}

test.describe('on a wide screen', () => {
	test.use({ viewport: { width: 1920, height: 1080 } })

	test('the label of a field row sits level with its buttons, in a one line row and in a repeater row', async ({
		page,
		request,
	}) => {
		const seeded = await seedFields(request)

		await openFields(page)

		await expectLevel(page, seeded.text)
		await expectLevel(page, seeded.repeater)
	})

	test('the kind and the API name of a field row sit level with its buttons', async ({ page, request }) => {
		const seeded = await seedFields(request)

		await openFields(page)

		const row = fieldRow(page, seeded.repeater)
		const trash = middle(await boxOf(trashOf(row, seeded.repeater)))
		const kind = middle(await boxOf(row.getByText('Repeater', { exact: true })))
		const name = middle(await boxOf(row.locator('code')))
		expect(Math.abs(kind - trash), 'the Kind badge is level with the trash').toBeLessThanOrEqual(1)
		expect(Math.abs(name - trash), 'the API name is level with the trash').toBeLessThanOrEqual(1)
	})
})

test.describe('on a phone', () => {
	test.use({ viewport: { width: 390, height: 844 } })

	test('a field row draws as a list item, its label level with its buttons and its values under them', async ({
		page,
		request,
	}) => {
		const seeded = await seedFields(request)

		await openFields(page)

		await expectLevel(page, seeded.text)
		await expectLevel(page, seeded.repeater)
		const row = fieldRow(page, seeded.text)
		const trash = await boxOf(trashOf(row, seeded.text))
		const kind = await boxOf(row.getByText('Text', { exact: true }))
		const name = await boxOf(row.locator('code'))
		expect(kind.y, 'the Kind badge sits under the first line').toBeGreaterThanOrEqual(trash.y + trash.height)
		expect(name.y, 'the API name sits under the first line').toBeGreaterThanOrEqual(trash.y + trash.height)
		const region = fieldRegion(page)
		const header = await region.locator('thead').evaluate((head) => {
			const { width, height } = head.getBoundingClientRect()
			return width * height
		})
		expect(header, 'the column headers take no room on screen').toBeLessThanOrEqual(1)
		const sideways = await region.evaluate((element) => element.scrollWidth - element.clientWidth)
		expect(sideways, 'the Fields region does not scroll sideways').toBeLessThanOrEqual(0)
	})
})

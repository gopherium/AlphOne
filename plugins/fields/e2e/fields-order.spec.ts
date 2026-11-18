// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { APIRequestContext, Page } from '@playwright/test'

import { graph } from '../../../test/e2e/graph'

/**
 * Defines one Text field through the graph.
 * @param request - The request context carrying the credential.
 * @param name - The field's API name.
 * @param label - The field's label.
 */
async function defineTextField(request: APIRequestContext, name: string, label: string) {
	await graph(
		request,
		'mutation($name: String!, $label: String!) { defineField(name: $name, label: $label, kind: TEXT) { id } }',
		{ name, label },
	)
}

/**
 * Waits for the graph to answer the order the Fields screen sends.
 * @param page - The page showing the Fields screen.
 * @returns The answer, once it arrives.
 */
function orderAnswer(page: Page) {
	return page.waitForResponse(
		(response) =>
			response.url().includes('/api/graphql') &&
			(response.request().postDataJSON() as { operationName?: string } | null)?.operationName === 'OrderFields',
	)
}

test('moves a field up, archives another, brings it back with Undo, and shows the new order on a contact', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const first = `Nickname ${stamp}`
	const second = `Shoe size ${stamp}`
	const third = `Hat size ${stamp}`
	await defineTextField(request, `nickname${stamp}`, first)
	await defineTextField(request, `shoeSize${stamp}`, second)
	await defineTextField(request, `hatSize${stamp}`, third)
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: `Maria Perez ${stamp}` },
	)

	await page.goto('/')
	await page.getByRole('link', { name: 'Fields' }).click()
	await expect(page.getByRole('heading', { name: 'Fields', level: 2 })).toBeVisible()
	const rows = page.getByRole('region', { name: 'Fields' }).getByRole('row').filter({ hasText: String(stamp) })
	await expect(rows).toContainText([first, second, third])
	const ordered = orderAnswer(page)
	await page.getByRole('button', { name: `Move ${second} up`, exact: true }).click()
	await expect(rows).toContainText([second, first, third])
	expect(await (await ordered).json()).toMatchObject({ data: { orderFields: true } })

	const middle = page.getByRole('button', { name: `Archive ${first}`, exact: true })
	await middle.click()
	await expect(rows).toHaveCount(2)
	await expect(rows).toContainText([second, third])
	const archived = page.locator('.godmin-toast').filter({ hasText: 'Field archived.' })
	await archived.getByRole('button', { name: 'Undo', exact: true }).click()
	await expect(page.locator('.godmin-toast').filter({ hasText: 'Field restored.' })).toBeVisible()
	await expect(rows).toHaveCount(3)
	await expect(rows).toContainText([second, first, third])
	await expect(middle).toBeFocused()
	await expect(page.locator('.godmin-toast').filter({ hasText: 'Could not order the fields.' })).toHaveCount(0)

	const trash = page.getByRole('button', { name: `Archive ${third}`, exact: true })
	await trash.click()
	await expect(rows).toHaveCount(2)
	await expect(rows).toContainText([second, first])
	await expect(rows.filter({ hasText: third })).toHaveCount(0)

	await page.goto(`/contacts/${created.createContact.id}`)
	const forms = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save fields' }) })
	const inputs = forms.getByRole('textbox', { name: String(stamp) })
	await expect(inputs).toHaveCount(2)
	await expect(inputs.nth(0)).toHaveAccessibleName(second)
	await expect(inputs.nth(1)).toHaveAccessibleName(first)
})

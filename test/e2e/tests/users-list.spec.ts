// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { APIRequestContext, Page } from '@playwright/test'

import { credentials } from '../env'
import { graph } from '../graph'
import { startMailSink } from '../mail'

/**
 * Invites one member per name, every address carrying a mark the list is searched by.
 * @param request - The Playwright request context carrying the credential.
 * @param mark - The mark every invited address carries.
 * @param names - The names of the accounts to invite.
 * @returns The addresses invited, in the order named.
 */
async function inviteAll(request: APIRequestContext, mark: string, names: string[]): Promise<string[]> {
	const sink = await startMailSink()
	try {
		const emails: string[] = []
		for (const [at, name] of names.entries()) {
			const email = `${mark}-${at}@example.com`
			await graph(
				request,
				'mutation($email: String!, $name: String!) { invite(email: $email, name: $name) { delivered } }',
				{ email, name },
			)
			emails.push(email)
		}
		return emails
	} finally {
		await sink.close()
	}
}

/**
 * Disables the account holding one address through the graph.
 * @param request - The Playwright request context carrying the credential.
 * @param email - The address of the account to disable.
 */
async function disable(request: APIRequestContext, email: string) {
	const listed = await graph<{ users: { id: string; email: string }[] }>(request, '{ users { id email } }')
	const held = listed.users.find((user) => user.email === email)
	expect(held, `an account holds ${email}`).toBeDefined()
	await graph(request, 'mutation($id: UUID!) { setUserDisabled(id: $id, disabled: true) }', { id: held?.id })
}

/**
 * Returns the row showing one address.
 * @param page - The page showing the users list.
 * @param email - The address the row shows.
 * @returns The row locator.
 */
function rowOf(page: Page, email: string) {
	return page.getByRole('row').filter({ hasText: email })
}

test('searches the users and narrows them by status through the filter button', async ({ page, request }) => {
	const mark = `list-${Date.now()}`
	const [invited, barred] = await inviteAll(request, mark, ['Maria Perez', 'Ana Lopez'])
	await disable(request, barred)

	await page.goto('/users')
	await page.getByRole('searchbox', { name: 'Search users…' }).fill(mark)

	await expect(rowOf(page, invited)).toBeVisible()
	await expect(rowOf(page, barred).getByText('Disabled')).toBeVisible()
	await expect(page).toHaveURL(new RegExp(`search=${mark}`))

	await page.getByRole('button', { name: 'Add filter' }).click()
	await page.getByRole('menuitem', { name: 'Status' }).click()
	await page.getByRole('option', { name: 'Invited' }).click()
	await page.keyboard.press('Escape')

	await expect(rowOf(page, invited).getByText('Invited', { exact: true })).toBeVisible()
	await expect(rowOf(page, barred)).toBeHidden()
})

test('pages the users newest first, and Back returns to the same page', async ({ page, request }) => {
	const mark = `page-${Date.now()}`
	const [oldest] = await inviteAll(request, mark, ['Ana Lopez', 'Luis Garcia', 'Maria Perez'])

	await page.goto(`/users?search=${mark}&perPage=2`)
	await expect(page.getByRole('row')).toHaveCount(3)
	await expect(rowOf(page, oldest)).toBeHidden()
	await page.getByRole('button', { name: 'Next page' }).click()

	await expect(rowOf(page, oldest)).toBeVisible()
	await expect(page).toHaveURL(/page=2/)

	const tabs = page.getByRole('navigation', { name: 'User sections' })
	await tabs.getByRole('link', { name: 'API tokens' }).click()
	await expect(tabs.getByRole('link', { name: 'API tokens' })).toHaveAttribute('aria-current', 'page')
	await expect(page.getByText('Manage the tokens your programs sign in with.')).toBeVisible()
	await expect(page.getByRole('heading', { level: 1, name: 'Users' })).toBeVisible()
	await page.goBack()

	await expect(page).toHaveURL(/page=2/)
	await expect(rowOf(page, oldest)).toBeVisible()
	await expect(page.getByRole('row')).toHaveCount(2)
})

test('changes the role of a user in a modal and confirms it with a toast', async ({ page, request }) => {
	const mark = `role-${Date.now()}`
	const [email] = await inviteAll(request, mark, ['Maria Perez'])

	await page.goto(`/users?search=${mark}`)
	const row = rowOf(page, email)
	await expect(row.getByRole('cell', { name: 'Member', exact: true })).toBeVisible()
	await row.getByRole('button', { name: 'Actions' }).click()
	await page.getByRole('menuitem', { name: 'Change role' }).click()
	const modal = page.getByRole('dialog', { name: 'Change role' })
	await expect(modal.getByText('Choose the role of Maria Perez.')).toBeVisible()
	await modal.getByRole('combobox', { name: 'Role' }).click()
	await page.getByRole('option', { name: 'Admin' }).click()
	await modal.getByRole('button', { name: 'Change role' }).click()

	await expect(page.locator('.godmin-toasts').getByText('Role changed.')).toBeVisible()
	await expect(modal).toBeHidden()
	await expect(row.getByRole('cell', { name: 'Admin', exact: true })).toBeVisible()
})

test('disables and enables several users at once, one toast each time', async ({ page, request }) => {
	const mark = `bulk-${Date.now()}`
	const [first, second] = await inviteAll(request, mark, ['Ana Lopez', 'Maria Perez'])

	await page.goto(`/users?search=${mark}`)
	await expect(page.getByText('2 Items')).toBeVisible()
	await page.getByRole('checkbox', { name: 'Ana Lopez' }).check()
	await page.getByRole('checkbox', { name: 'Maria Perez' }).check()
	await page.getByRole('button', { name: 'Disable', exact: true }).click()

	await expect(page.locator('.godmin-toasts').getByText('2 users disabled.')).toBeVisible()
	await expect(rowOf(page, first).getByText('Disabled')).toBeVisible()
	await expect(rowOf(page, second).getByText('Disabled')).toBeVisible()

	await expect(page.getByText('2 Items selected')).toBeVisible()
	await page.getByRole('button', { name: 'Enable', exact: true }).click()

	await expect(page.locator('.godmin-toasts').getByText('2 users enabled.')).toBeVisible()
	await expect(rowOf(page, first).getByText('Invited', { exact: true })).toBeVisible()
})

test.describe('on a phone', () => {
	test.use({ viewport: { width: 390, height: 844 } })

	test('lays the users out as a list rather than a table', async ({ page }) => {
		await page.goto(`/users?search=${encodeURIComponent(credentials.email)}`)

		await expect(page.getByRole('heading', { level: 1, name: 'Users' })).toBeVisible()
		await expect(page.getByText(credentials.email).first()).toBeVisible()
		await expect(page.locator('table')).toHaveCount(0)
	})
})

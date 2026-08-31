// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { linkFrom, startMailSink } from '../mail'
import type { MailSink } from '../mail'

let sink: MailSink

test.beforeAll(async () => {
	sink = await startMailSink()
})

test.afterAll(async () => {
	await sink.close()
})

test.beforeEach(() => {
	sink.clear()
})

/**
 * Invites one person from the new user screen.
 * @param page - The signed-in admin's page.
 * @param email - The address to invite.
 * @param name - The name the invitation greets.
 */
async function invite(page: import('@playwright/test').Page, email: string, name: string) {
	await page.goto('/users/new')
	await page.getByLabel('Email').fill(email)
	await page.getByLabel('Name').fill(name)
	await page.getByRole('button', { name: 'Send invitation' }).click()
}

test('an invited person activates from the mailed link and lands on their tasks', async ({
	page,
	browser,
}) => {
	await invite(page, 'ada@example.com', 'Ada Lovelace')
	await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible()

	const body = await sink.waitFor('ada@example.com')
	const link = linkFrom(body, '/activate')

	const anonymous = await browser.newContext({ storageState: { cookies: [], origins: [] } })
	const activation = await anonymous.newPage()
	await activation.goto(link)
	await activation.getByLabel('Password').fill('correct horse battery')
	await activation.getByRole('button', { name: 'Set password' }).click()

	await expect(activation.getByRole('heading', { name: 'Tasks' })).toBeVisible()
	await anonymous.close()
})

test('the users list marks an invited account and resends its invitation', async ({ page }) => {
	await invite(page, 'pending@example.com', 'Maria Perez')
	await sink.waitFor('pending@example.com')
	sink.clear()

	await page.goto('/users')
	const row = page.getByRole('row').filter({ hasText: 'pending@example.com' })
	await expect(row.getByText('Invited', { exact: true })).toBeVisible()

	await row.getByRole('button', { name: 'Resend invitation to Maria Perez' }).click()

	await expect(row.getByText('Invitation sent.')).toBeVisible()
	const resent = await sink.waitFor('pending@example.com')
	expect(resent).toContain('/activate?token=')
})

test('a forgotten password is reset from the login screen', async ({ page, browser }) => {
	await invite(page, 'maria@example.com', 'Maria Perez')
	const invitation = await sink.waitFor('maria@example.com')
	sink.clear()

	const anonymous = await browser.newContext({ storageState: { cookies: [], origins: [] } })
	const activation = await anonymous.newPage()
	await activation.goto(linkFrom(invitation, '/activate'))
	await activation.getByLabel('Password').fill('correct horse battery')
	await activation.getByRole('button', { name: 'Set password' }).click()
	await expect(activation.getByRole('heading', { name: 'Tasks' })).toBeVisible()
	await anonymous.close()

	const forgotten = await browser.newContext({ storageState: { cookies: [], origins: [] } })
	const reset = await forgotten.newPage()
	await reset.goto('/')

	await reset.getByRole('button', { name: 'Forgot your password?' }).click()
	await reset.getByLabel('Email').fill('maria@example.com')
	await reset.getByRole('button', { name: 'Send reset link' }).click()
	await expect(
		reset.getByText('If that address has an account, a reset link is on its way.'),
	).toBeVisible()

	const body = await sink.waitFor('maria@example.com')
	await reset.goto(linkFrom(body, '/reset-password'))
	await reset.getByLabel('Password').fill('a brand new password')
	await reset.getByRole('button', { name: 'Set password' }).click()

	await expect(reset.getByText('Your password is set. Sign in with it.')).toBeVisible()

	await reset.getByRole('button', { name: 'Go to login' }).click()
	await reset.getByLabel('Email').fill('maria@example.com')
	await reset.getByLabel('Password').fill('a brand new password')
	await reset.getByRole('button', { name: 'Log in' }).click()

	await expect(reset.getByRole('heading', { name: 'Tasks' })).toBeVisible()
	await forgotten.close()
})

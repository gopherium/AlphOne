// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { baseURL } from '../env'

test('a member reads the account list without managing it', async ({ page, browser }) => {
	const stamp = Date.now()
	const email = `staff-${stamp}@example.com`
	const name = `Maria Perez ${stamp}`
	const password = 'correct horse battery'

	await page.goto('/users')
	await page.getByRole('link', { name: 'New user' }).click()
	await page.getByLabel('Email').fill(email)
	await page.getByLabel('Name').fill(name)
	await page.getByLabel('Password').fill(password)
	await page.getByRole('button', { name: 'Create user' }).click()

	const created = page.getByRole('row').filter({ hasText: email })
	await expect(created.getByRole('cell', { name: 'Member', exact: true })).toBeVisible()

	const member = await browser.newContext({
		baseURL,
		storageState: { cookies: [], origins: [] },
	})
	const memberPage = await member.newPage()
	await memberPage.goto('/')
	await memberPage.getByLabel('Email').fill(email)
	await memberPage.getByLabel('Password').fill(password)
	await memberPage.getByRole('button', { name: 'Log in' }).click()
	await expect(memberPage.getByRole('heading', { name: 'Tasks' })).toBeVisible()

	await memberPage.goto('/users')

	await expect(memberPage.getByRole('row').filter({ hasText: email })).toBeVisible()
	await expect(memberPage.getByRole('link', { name: 'New user' })).toBeHidden()
	await expect(memberPage.getByRole('button', { name: /^Disable / })).toBeHidden()
	await expect(memberPage.getByRole('button', { name: /^Promote / })).toBeHidden()

	await member.close()
})

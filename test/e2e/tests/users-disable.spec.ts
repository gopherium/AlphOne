// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { baseURL } from '../env'

test('disabling a user revokes their live session', async ({ page, browser }) => {
	const stamp = Date.now()
	const email = `victim-${stamp}@example.com`
	const name = `Victim ${stamp}`
	const password = 'correct horse battery'

	await page.goto('/users')
	await page.getByRole('link', { name: 'New user' }).click()
	await page.getByLabel('Email').fill(email)
	await page.getByLabel('Name').fill(name)
	await page.getByLabel('Password').fill(password)
	await page.getByRole('button', { name: 'Create user' }).click()

	const row = page.getByRole('row').filter({ hasText: email })
	await expect(row.getByText('Active')).toBeVisible()

	const victim = await browser.newContext({
		baseURL,
		storageState: { cookies: [], origins: [] },
	})
	const victimPage = await victim.newPage()
	await victimPage.goto('/')
	await victimPage.getByLabel('Email').fill(email)
	await victimPage.getByLabel('Password').fill(password)
	await victimPage.getByRole('button', { name: 'Log in' }).click()
	await expect(victimPage.getByText('Welcome to AlphOne.')).toBeVisible()

	await page.getByRole('button', { name: `Disable ${name}` }).click()
	await expect(row.getByText('Disabled')).toBeVisible()

	await victimPage.reload()

	await expect(victimPage.getByLabel('Email')).toBeVisible()
	await expect(victimPage.getByText('Welcome to AlphOne.')).toBeHidden()

	await victim.close()
})

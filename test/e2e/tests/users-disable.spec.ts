// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { provisionUser } from '../provision'

import { baseURL } from '../env'

test('disabling a user revokes their live session', async ({ page, browser }) => {
	const stamp = Date.now()
	const email = `victim-${stamp}@example.com`
	const name = `Victim ${stamp}`
	const password = 'correct horse battery'

	await provisionUser(page, browser, { email, name, password })

	await page.goto('/users')

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
	await expect(victimPage.getByRole('heading', { name: 'Tasks' })).toBeVisible()

	await page.getByRole('button', { name: `Disable ${name}` }).click()
	await expect(row.getByText('Disabled')).toBeVisible()

	await victimPage.reload()

	await expect(victimPage.getByLabel('Email')).toBeVisible()
	await expect(victimPage.getByRole('heading', { name: 'Tasks' })).toBeHidden()

	await victim.close()
})

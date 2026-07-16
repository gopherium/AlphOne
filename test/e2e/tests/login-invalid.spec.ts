// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { credentials } from '../env'

test.use({ storageState: { cookies: [], origins: [] } })

test('rejects a wrong password without starting a session', async ({ page }) => {
	await page.goto('/')
	await page.getByLabel('Email').fill(credentials.email)
	await page.getByLabel('Password').fill('not the right password')

	await page.getByRole('button', { name: 'Log in' }).click()

	await expect(page.getByRole('alert')).toHaveText('Invalid email or password.')
	await expect(page.getByRole('button', { name: 'Log out' })).toBeHidden()

	await page.reload()

	await expect(page.getByLabel('Email')).toBeVisible()
})

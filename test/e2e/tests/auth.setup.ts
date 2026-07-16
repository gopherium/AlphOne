// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test as setup } from '@playwright/test'

import { authFile, credentials } from '../env'

setup('logs in and stores the session', async ({ page }) => {
	await page.goto('/')

	await page.getByLabel('Email').fill(credentials.email)
	await page.getByLabel('Password').fill(credentials.password)
	await page.getByRole('button', { name: 'Log in' }).click()

	await expect(page.getByText('Welcome to AlphOne.')).toBeVisible()
	await expect(page.getByText(credentials.name)).toBeVisible()

	await page.context().storageState({ path: authFile })
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { credentials } from '../env'

test.use({ storageState: { cookies: [], origins: [] } })

test('logs out and cannot be restored by reloading', async ({ page }) => {
	await page.goto('/')
	await page.getByLabel('Email').fill(credentials.email)
	await page.getByLabel('Password').fill(credentials.password)
	await page.getByRole('button', { name: 'Log in' }).click()
	await expect(page.getByRole('button', { name: 'Log out' })).toBeVisible()

	await page.getByRole('button', { name: 'Log out' }).click()

	await expect(page.getByLabel('Email')).toBeVisible()
	await expect(page.getByRole('button', { name: 'Log out' })).toBeHidden()

	await page.reload()

	await expect(page.getByLabel('Email')).toBeVisible()
})

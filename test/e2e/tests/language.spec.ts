// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { credentials } from '../env'

test.use({ storageState: { cookies: [], origins: [] } })

/** reader is the second account, so the shared one keeps reading English. */
const reader = {
	email: `e2e-reader-${Date.now()}@example.com`,
	name: 'Maria Perez',
	password: 'correct horse battery',
}

test('reads the interface in the language the reader chose', async ({ page, browser }) => {
	await page.goto('/')
	await page.getByLabel('Email').fill(credentials.email)
	await page.getByLabel('Password').fill(credentials.password)
	await page.getByRole('button', { name: 'Log in' }).click()
	await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

	await page.goto('/users/new')
	await page.getByLabel('Email').fill(reader.email)
	await page.getByLabel('Name').fill(reader.name)
	await page.getByLabel('Password').fill(reader.password)
	await page.getByRole('button', { name: 'Create user' }).click()
	await expect(page.getByText(reader.email)).toBeVisible()

	const context = await browser.newContext({ storageState: { cookies: [], origins: [] } })
	const theirs = await context.newPage()
	await theirs.goto('/')
	await theirs.getByLabel('Email').fill(reader.email)
	await theirs.getByLabel('Password').fill(reader.password)
	await theirs.getByRole('button', { name: 'Log in' }).click()
	await expect(theirs.getByRole('heading', { name: 'Tasks' })).toBeVisible()

	await theirs.goto('/language')
	await theirs.getByRole('combobox', { name: 'Language' }).click()
	await theirs.getByRole('option', { name: 'es-ES' }).click()
	await theirs.getByRole('button', { name: 'Save' }).click()
	await expect(theirs.getByRole('status')).toBeVisible()

	await theirs.reload()

	await expect(theirs.getByRole('heading', { name: 'Idioma' })).toBeVisible()
	await expect(theirs.getByRole('button', { name: 'Guardar' })).toBeVisible()
	await expect(theirs.getByRole('navigation', { name: 'Navegación' })).toBeVisible()

	await expect(page.getByRole('navigation', { name: 'Navigation' })).toBeVisible()
	await context.close()
})

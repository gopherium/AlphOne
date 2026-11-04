// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

import { credentials, legalAddresses } from '../env'

test.use({ storageState: { cookies: [], origins: [] } })

/**
 * Expects the two legal links on the page, the terms first, each leading to its address.
 * @param page - The page holding the links.
 */
async function expectLegalLinks(page: Page) {
	await expect(page.locator('.gopherium-login__links')).toHaveText('Terms of service | Privacy policy')
	const terms = page.getByRole('link', { name: 'Terms of service' })
	await expect(terms).toHaveAttribute('href', legalAddresses.terms)
	await expect(terms).toHaveAttribute('rel', 'noopener noreferrer terms-of-service')
	const privacy = page.getByRole('link', { name: 'Privacy policy' })
	await expect(privacy).toHaveAttribute('href', legalAddresses.privacy)
	await expect(privacy).toHaveAttribute('rel', 'noopener noreferrer privacy-policy')
}

test('the sign-in screen links to the legal pages', async ({ page }) => {
	await page.goto('/')
	await expect(page.getByRole('button', { name: 'Log in' })).toBeVisible()

	await expectLegalLinks(page)
})

test('the lost password screen links to the legal pages', async ({ page }) => {
	await page.goto('/')
	await page.getByRole('button', { name: 'Lost your password?' }).click()
	await expect(page.getByRole('button', { name: 'Send reset link' })).toBeVisible()

	await expectLegalLinks(page)
})

test('the set password screen links to the legal pages', async ({ page }) => {
	await page.goto('/activate?token=not-a-real-token')
	await expect(page.getByRole('button', { name: 'Set password' })).toBeVisible()

	await expectLegalLinks(page)
})

test('the reset password screen links to the legal pages', async ({ page }) => {
	await page.goto('/reset-password?token=not-a-real-token')
	await expect(page.getByRole('button', { name: 'Set password' })).toBeVisible()

	await expectLegalLinks(page)
})

test('a reader who opened the app signed in sees the links after logging out', async ({ page }) => {
	await page.goto('/')
	await page.getByLabel('Email').fill(credentials.email)
	await page.getByLabel('Password').fill(credentials.password)
	await page.getByRole('button', { name: 'Log in' }).click()
	await expect(page.getByRole('button', { name: 'Log out' })).toBeVisible()

	await page.reload()
	await page.getByRole('button', { name: 'Log out' }).click()
	await expect(page.getByLabel('Email')).toBeVisible()

	await expectLegalLinks(page)
})

test('the links sit under the form', async ({ page }) => {
	await page.goto('/')
	const button = await page.getByRole('button', { name: 'Log in' }).boundingBox()
	const links = await page.locator('.gopherium-login__links').boundingBox()

	expect(links?.y).toBeGreaterThan(button?.y ?? Number.POSITIVE_INFINITY)
})

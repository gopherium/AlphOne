// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

import { graph } from '../graph'

/**
 * Counts the documents the page loads in its main frame.
 * @param page - The page to watch.
 * @returns A function giving the number of documents loaded so far.
 */
function documentsLoaded(page: Page): () => number {
	let documents = 0
	page.on('request', (request) => {
		if (request.isNavigationRequest() && request.frame() === page.mainFrame()) {
			documents += 1
		}
	})
	return () => documents
}

/**
 * Fails every request for one lazy chunk until the page loads again, like a tab opened before a release.
 * @param page - The page that loses the chunk.
 * @param prefix - The chunk name before its hash.
 */
async function loseChunkUntilReload(page: Page, prefix: string) {
	const documents = documentsLoaded(page)
	await page.route(`**/assets/${prefix}*.js`, (route) => (documents() < 2 ? route.abort() : route.continue()))
}

/**
 * Fails every request for one lazy chunk, like a chunk a release removed for good.
 * @param page - The page that loses the chunk.
 * @param prefix - The chunk name before its hash.
 * @returns A function giving the number of documents loaded so far.
 */
async function loseChunkForGood(page: Page, prefix: string): Promise<() => number> {
	const documents = documentsLoaded(page)
	await page.route(`**/assets/${prefix}*.js`, (route) => route.abort())
	return documents
}

test('the contact page reloads once and shows the fields panel when its chunk is gone', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const name = `Maria Perez ${stamp}`
	const label = `Nickname ${stamp}`
	await graph(
		request,
		'mutation($name: String!, $label: String!) { defineField(name: $name, label: $label, kind: TEXT) { id } }',
		{ name: `nickname${stamp}`, label },
	)
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name },
	)
	await loseChunkUntilReload(page, 'ContactFieldsPanel-')

	await page.goto(`/contacts/${created.createContact.id}`)

	await expect(page.getByRole('heading', { name, level: 2 })).toBeVisible()
	await expect(page.getByRole('textbox', { name: label, exact: true })).toBeVisible()
})

test('the WhatsApp section reloads once and shows its sidebar when the sidebar chunk is gone', async ({
	page,
}) => {
	await loseChunkUntilReload(page, 'WhatsAppSidebar-')

	await page.goto('/whatsapp')

	const nav = page.getByRole('region', { name: 'Navigation' })
	await expect(nav.getByRole('heading', { level: 1, name: 'WhatsApp' })).toBeVisible()
	await expect(page.getByText('No conversation selected.')).toBeVisible()
})

test('the WhatsApp section reloads only once and then shows the error when the sidebar chunk stays gone', async ({
	page,
}) => {
	const documents = await loseChunkForGood(page, 'WhatsAppSidebar-')

	await page.goto('/whatsapp')

	await expect(page.getByText('Something went wrong!')).toBeVisible()
	expect(documents()).toBe(2)
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'

import { expect, test } from '@playwright/test'

import { baseURL } from '../env'
import { graph } from '../graph'

/** A page served from another origin, and how to stop serving it. */
type OtherSite = { url: string; close: () => Promise<void> }

/** The hosts another page stands on, each with what the browser says about it. */
const otherSites = [
	{ host: '127.0.0.1', fetchSite: 'cross-site' },
	{ host: 'localhost', fetchSite: 'same-site' },
]

/**
 * Escapes text for a double quoted HTML attribute.
 * @param text - The raw text.
 * @returns The text with its markup characters escaped.
 */
function attribute(text: string): string {
	return text.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;')
}

/**
 * Returns a page whose form posts a new contact to the graph as multipart.
 * @param name - The name of the contact the form asks for.
 * @returns The page markup.
 */
function formPage(name: string): string {
	const operations = JSON.stringify({
		query: 'mutation($name: String!) { createContact(name: $name) { id } }',
		variables: { name },
	})
	return [
		'<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Another site</title></head><body>',
		`<form method="post" enctype="multipart/form-data" action="${attribute(`${baseURL}/api/graphql`)}">`,
		`<input type="hidden" name="operations" value="${attribute(operations)}">`,
		'<input type="hidden" name="map" value="{}">',
		'<button type="submit">Send</button>',
		'</form></body></html>',
	].join('')
}

/**
 * Serves the form page on a port of its own, visited through host.
 * @param host - The host name the browser visits the page on.
 * @param name - The name of the contact the form asks for.
 * @returns The page address and a way to stop serving it.
 */
async function serveOtherSite(host: string, name: string): Promise<OtherSite> {
	const server = createServer((_request, response) => {
		response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
		response.end(formPage(name))
	})
	await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
	const { port } = server.address() as AddressInfo
	return {
		url: `http://${host}:${port}/`,
		close: () => new Promise<void>((resolve) => server.close(() => resolve())),
	}
}

for (const { host, fetchSite } of otherSites) {
	test(`a form served from ${host} on another port is refused and creates no contact`, async ({
		page,
		request,
	}) => {
		const name = `Maria Perez ${Date.now()}`
		const site = await serveOtherSite(host, name)
		try {
			await page.goto(site.url)
			const posted = page.waitForResponse(
				(response) => response.url() === `${baseURL}/api/graphql` && response.request().method() === 'POST',
			)
			await page.getByRole('button', { name: 'Send', exact: true }).click()
			const response = await posted

			expect.soft(await response.request().headerValue('origin')).toBe(new URL(site.url).origin)
			expect.soft(await response.request().headerValue('sec-fetch-site')).toBe(fetchSite)
			expect.soft(response.status(), 'the app took the write').toBe(403)
			expect.soft(await response.json()).toEqual({
				error: 'cross-origin request refused',
				code: 'request_cross_origin',
			})

			const stored = await graph<{ contacts: { edges: { node: { name: string } }[] } }>(
				request,
				'query($q: String) { contacts(q: $q) { edges { node { name } } } }',
				{ q: name },
			)
			expect(stored.contacts.edges, 'the form created the contact').toEqual([])
		} finally {
			await site.close()
		}
	})
}

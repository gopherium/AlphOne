// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { startMailSink } from '../mail'
import type { APIRequestContext } from '@playwright/test'

import { createTask, graph, graphUpload } from '../graph'
import { deliverInboundText } from '../inbound'

test.use({ viewport: { width: 390, height: 844 } })

/**
 * Seeds one of every record a detail route needs, and returns their paths.
 * @param request - The Playwright request context.
 * @returns Every route the sweep visits.
 */
async function seedRoutes(request: APIRequestContext): Promise<string[]> {
	const stamp = Date.now()
	const unbreakable = 'W'.repeat(60)
	const task = await createTask(request, {
		title: `Approve the revised pricing schedule before the renewal closes ${stamp}`,
		dueOn: new Date().toISOString().slice(0, 10),
	})
	const contact = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: `Maria Perez de la Fuente y Rodriguez ${stamp}` },
	)
	const sink = await startMailSink()
	try {
		await graph(
			request,
			'mutation($email: String!, $name: String!) {' +
				' invite(email: $email, name: $name) { delivered } }',
			{
				email: `maria.perez.de.la.fuente.${stamp}@example.com`,
				name: `Maria Perez de la Fuente ${stamp}`,
			},
		)
	} finally {
		await sink.close()
	}

	await deliverInboundText(request, `1999${stamp}`, `Ada ${stamp}`, unbreakable)
	const conversations = await graph<{ whatsAppConversations: { id: string }[] }>(
		request,
		'{ whatsAppConversations { id } }',
	)

	const staged = await graphUpload<{ importUpload: { id: string } }>(
		request,
		'mutation($file: Upload!) { importUpload(file: $file) { id } }',
		{
			name: `contacts-${stamp}.csv`,
			mimeType: 'text/csv',
			buffer: Buffer.from(
				`Name,Email address of the person being imported\n` +
					`Maria Perez de la Fuente ${stamp},${unbreakable}@example.com\n`,
			),
		},
	)
	const importID = staged.importUpload.id

	const taskID = task.id
	const contactID = contact.createContact.id
	const conversationID = conversations.whatsAppConversations[0].id

	return [
		'/tasks',
		'/tasks/new',
		`/tasks/${taskID}`,
		'/contacts',
		'/contacts/new',
		`/contacts/${contactID}`,
		'/users',
		'/users/new',
		'/whatsapp',
		`/whatsapp/conversations/${conversationID}`,
		'/import',
		`/import/${importID}`,
	]
}

test('every screen fits a phone without spilling sideways', async ({ page, request }) => {
	const routes = await seedRoutes(request)
	const spills: string[] = []

	for (const route of routes) {
		await page.goto(route)
		await expect(page.locator('main h1')).toBeVisible()
		await expect(page.getByText(/^Loading/)).toHaveCount(0)

		const report = await page.evaluate(() => {
			const canvas = document.querySelector('.godmin-layout__canvas')
			if (canvas === null) {
				return { documentSpills: false, canvasSpills: false, widest: [] as string[] }
			}
			const limit = canvas.getBoundingClientRect().right

			const scrolls = (element: Element) => {
				let parent = element.parentElement
				while (parent !== null && parent !== canvas.parentElement) {
					const overflow = getComputedStyle(parent).overflowX
					if (overflow === 'auto' || overflow === 'scroll') {
						return true
					}
					parent = parent.parentElement
				}
				return false
			}
			const widest = [...canvas.querySelectorAll('*')]
				.filter((element) => element.getBoundingClientRect().right > limit + 1)
				.filter((element) => !scrolls(element))
				.map((element) => `${element.tagName.toLowerCase()}.${element.className}`)
			return {
				documentSpills:
					document.documentElement.scrollWidth > document.documentElement.clientWidth,
				canvasSpills: canvas.scrollWidth > canvas.clientWidth,
				widest: widest.slice(0, 3),
			}
		})

		if (report.documentSpills || report.canvasSpills || report.widest.length > 0) {
			spills.push(`${route}: ${JSON.stringify(report)}`)
		}
	}

	expect(spills).toEqual([])
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { cleanup, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, expect, test } from 'vitest'

import { importID, handlers as importerHandlers } from '@alphone/plugin-importer/handlers'
import { handlers } from '@alphone/plugin-whatsapp/handlers'
import { createAppRouter } from '../router'
import { renderAt } from './render'

const taskID = '0198c000-0000-7000-8000-000000000301'
const contactID = '0198c000-0000-7000-8000-000000000302'
const conversationID = '019f4a00-0000-7000-8000-000000000001'

// Every leaf route, with its parameters bound to a fixture the handlers serve.
const paths: Record<string, string> = {
	'/': '/',
	'/tasks': '/tasks',
	'/tasks/new': '/tasks/new',
	'/tasks/$taskId': `/tasks/${taskID}`,
	'/contacts': '/contacts',
	'/contacts/new': '/contacts/new',
	'/contacts/$contactId': `/contacts/${contactID}`,
	'/users': '/users',
	'/users/new': '/users/new',
	// The section route holds the sidebar and renders its canvas through the
	// index child, so both ids resolve to the same URL.
	'/whatsapp': '/whatsapp',
	'/whatsapp/': '/whatsapp',
	'/whatsapp/conversations/$conversationId': `/whatsapp/conversations/${conversationID}`,
	'/import': '/import',
	'/import/$importId': `/import/${importID}`,
}

beforeEach(() => {
	server.use(
		...handlers,
		...importerHandlers,
		http.get('/api/tasks/:id', () =>
			HttpResponse.json({
				id: taskID,
				assignee_id: '0198c000-0000-7000-8000-0000000000aa',
				contact_id: null,
				title: 'Call the supplier',
				status: 'open',
				priority: 0,
				due_on: '2026-08-10',
				origin_source: null,
				origin_event_id: null,
				created_at: '2026-07-29T10:00:00Z',
			}),
		),
		http.get('/api/contacts/:id', () =>
			HttpResponse.json({
				id: contactID,
				name: 'Maria Perez',
				created_at: '2026-07-29T10:00:00Z',
				identities: [],
			}),
		),
	)
})

test('every leaf route is covered by the outline invariant', () => {
	const leaves = Object.keys(createAppRouter().routesById)
		.filter((id) => id !== '__root__')
		.sort()

	expect(leaves).toEqual(Object.keys(paths).sort())
})

/**
 * Returns the first level headings the canvas is showing.
 * @returns The heading texts.
 */
function firstLevelHeadings(): (string | null)[] {
	return within(screen.getByRole('main'))
		.queryAllByRole('heading')
		.filter((heading) => heading.tagName === 'H1')
		.map((heading) => heading.textContent)
}

test.each(Object.entries(paths))(
	'route %s renders exactly one first level heading',
	async (_id, path) => {
		renderAt(path)
		await screen.findByRole('main')
		await waitFor(() => expect(firstLevelHeadings()).toHaveLength(1))
		await new Promise((resolve) => setTimeout(resolve, 400))

		expect(firstLevelHeadings()).toHaveLength(1)
		cleanup()
	},
)

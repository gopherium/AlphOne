// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { cleanup, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'

vi.mock('@alphone/frontend-sdk/dataviews', () => ({
	DataViews: () => <table />,
	DataForm: () => <form />,
}))

import { createAppRouter } from '../router'
import { renderAt } from './render'

const taskID = '0198c000-0000-7000-8000-000000000301'
const contactID = '0198c000-0000-7000-8000-000000000302'

/** PluginOutline is the leaf routes and fixtures one plugin contributes to the sweep. */
interface PluginOutline {
	paths: Record<string, string>
	handlers?: Parameters<typeof server.use>
}

/** pluginOutlines holds every plugin's outline from both plugin roots. */
const pluginOutlines = Object.values(
	import.meta.glob(
		[
			'../../../plugins/*/frontend/outline.ts',
			'../../../enterprise/*/frontend/outline.ts',
		],
		{ eager: true },
	),
) as PluginOutline[]

/** corePaths is every core leaf route, its parameters bound to a served fixture. */
const corePaths: Record<string, string> = {
	'/': '/',
	'/tasks': '/tasks',
	'/tasks/new': '/tasks/new',
	'/tasks/$taskId': `/tasks/${taskID}`,
	'/contacts': '/contacts',
	'/contacts/new': '/contacts/new',
	'/contacts/$contactId': `/contacts/${contactID}`,
	'/users': '/users',
	'/users/new': '/users/new',
	'/users/tokens': '/users/tokens',
	'/users/tokens/new': '/users/tokens/new',
	'/language': '/language',
}

/** paths is every leaf route of the composed router, core plus both plugin roots. */
const paths: Record<string, string> = Object.assign(
	{},
	corePaths,
	...pluginOutlines.map((outline) => outline.paths),
)

beforeEach(() => {
	server.use(
		...pluginOutlines.flatMap((outline) => outline.handlers ?? []),
		graphql.query('TaskDetail', () =>
			HttpResponse.json({
				data: {
					task: {
						__typename: 'Task',
						id: taskID,
						title: 'Call the supplier',
						status: 'open',
						priority: 0,
						dueOn: '2026-08-10',
						contactId: null,
						contact: null,
					},
				},
			}),
		),
		graphql.query('ContactDetail', () =>
			HttpResponse.json({
				data: {
					contact: {
						__typename: 'Contact',
						id: contactID,
						name: 'Maria Perez',
						createdAt: '2026-07-29T10:00:00Z',
						identities: [],
						tasks: {
							__typename: 'TaskConnection',
							edges: [],
							pageInfo: { __typename: 'PageInfo', hasNextPage: false, endCursor: null },
						},
					},
				},
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

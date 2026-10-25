// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { cleanup, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'

vi.mock('@alphone/frontend-sdk/dataviews', () => ({
	DataViews: () => <table />,
	DataForm: () => <form />,
	filterSortAndPaginate: (data: unknown[]) => ({ data, paginationInfo: { totalItems: data.length, totalPages: 1 } }),
	useEnableWpCompatOverlaySlot: () => {},
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
	'/admin/users': '/users',
	'/admin/users/new': '/users/new',
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
						dueTime: null,
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
	const leaves = Object.values(createAppRouter().routesById)
		.filter((route) => route.id !== '__root__' && route.path !== undefined)
		.map((route) => route.id)
		.sort()

	expect(leaves).toEqual(Object.keys(paths).sort())
})

/**
 * Returns the headings of one level the canvas is showing.
 * @param tag - The tag of the heading level, such as H2.
 * @returns The heading texts.
 */
function canvasHeadings(tag: string): (string | null)[] {
	return within(screen.getByRole('main'))
		.queryAllByRole('heading')
		.filter((heading) => heading.tagName === tag)
		.map((heading) => heading.textContent)
}

test.each(Object.entries(paths))(
	'route %s titles its page at the second level, under the sidebar title',
	async (_id, path) => {
		renderAt(path)
		await screen.findByRole('main')
		await waitFor(() => expect(canvasHeadings('H2')).toHaveLength(1))
		await new Promise((resolve) => setTimeout(resolve, 400))

		expect(canvasHeadings('H2')).toHaveLength(1)
		expect(canvasHeadings('H1')).toEqual([])
		const sidebar = screen.getByRole('region', { name: 'Navigation' })
		const firstLevel = screen.getAllByRole('heading').filter((heading) => heading.tagName === 'H1')
		expect(firstLevel).toHaveLength(1)
		expect(sidebar).toContainElement(firstLevel[0])
		cleanup()
	},
)

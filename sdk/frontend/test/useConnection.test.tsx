// SPDX-License-Identifier: AGPL-3.0-or-later

import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { gql } from 'urql'
import { expect, test, vi } from 'vitest'

import { GraphProvider } from '../GraphProvider'
import { createGraphClient } from '../graph'
import { HttpResponse, graphql, server } from '../testing'
import { useConnection } from '../useConnection'

const contactsQuery = gql`
	query Contacts($q: String, $first: Int, $after: String) {
		contacts(q: $q, first: $first, after: $after) {
			edges {
				node {
					id
					name
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
		}
	}
`

interface ContactNode {
	id: string
	name: string
}

/** Returns one contacts page as the graph serializes it. */
function page(node: ContactNode, hasNextPage: boolean, endCursor: string) {
	return {
		__typename: 'ContactConnection',
		edges: [
			{
				__typename: 'ContactEdge',
				node: { __typename: 'Contact', ...node },
				cursor: endCursor,
			},
		],
		pageInfo: { __typename: 'PageInfo', hasNextPage, endCursor },
	}
}

/** Answers the contacts query from a cursor keyed set of pages. */
function servePages(pages: Record<string, ReturnType<typeof page>>) {
	const served: (string | undefined)[] = []
	server.use(
		graphql.query('Contacts', ({ variables }) => {
			served.push(variables.after as string | undefined)
			const key = (variables.after as string | undefined) ?? 'first'
			return HttpResponse.json({ data: { contacts: pages[key] } })
		}),
	)
	return served
}

/** Renders the hook inside a fresh graph client. */
function renderConnection(variables: Record<string, unknown> = {}) {
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	function Wrapper({ children }: { children: ReactNode }) {
		return <GraphProvider graph={graph}>{children}</GraphProvider>
	}
	return renderHook(
		(props: { variables: Record<string, unknown> }) =>
			useConnection({
				query: contactsQuery,
				variables: props.variables,
				select: (data: { contacts: unknown }) =>
					data.contacts as { edges: { node: ContactNode }[]; pageInfo: never },
			}),
		{ wrapper: Wrapper, initialProps: { variables } },
	)
}

test('exposes the rows of the first page', async () => {
	servePages({ first: page({ id: 'id-ada', name: 'Ada Lovelace' }, false, 'cursor-ada') })

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.isPending).toBe(false))
	expect(result.current.rows.map((row) => row.name)).toEqual(['Ada Lovelace'])
	expect(result.current.hasNextPage).toBe(false)
	expect(result.current.isError).toBe(false)
})

test('appends the next page and stops offering more at the end', async () => {
	const served = servePages({
		first: page({ id: 'id-ada', name: 'Ada Lovelace' }, true, 'cursor-ada'),
		'cursor-ada': page({ id: 'id-maria', name: 'Maria Perez' }, false, 'cursor-maria'),
	})

	const { result } = renderConnection({ first: 50 })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.rows).toHaveLength(2))
	expect(result.current.rows.map((row) => row.name)).toEqual(['Ada Lovelace', 'Maria Perez'])
	expect(result.current.hasNextPage).toBe(false)
	expect(served).toEqual([undefined, 'cursor-ada'])
})

test('reports the failure when the page cannot be loaded', async () => {
	server.use(
		graphql.query('Contacts', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'boom' }] }),
		),
	)

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.isError).toBe(true))
	expect(result.current.rows).toEqual([])
	expect(result.current.isPending).toBe(false)
})

test('pages from the start again when the variables change', async () => {
	const served = servePages({
		first: page({ id: 'id-ada', name: 'Ada Lovelace' }, true, 'cursor-ada'),
		'cursor-ada': page({ id: 'id-maria', name: 'Maria Perez' }, false, 'cursor-maria'),
	})
	const { result, rerender } = renderConnection({ first: 50, q: '' })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(2))

	rerender({ variables: { first: 50, q: 'ada' } })

	await waitFor(() => expect(result.current.rows).toHaveLength(1))
	expect(served.at(-1)).toBeUndefined()
})

test('runs no query while it is paused', async () => {
	const served = servePages({ first: page({ id: 'id-ada', name: 'Ada Lovelace' }, false, 'c') })
	const graph = createGraphClient({ onSessionExpired: vi.fn() })

	const { result } = renderHook(
		() =>
			useConnection({
				query: contactsQuery,
				variables: { first: 50 },
				pause: true,
				select: (data: { contacts: unknown }) =>
					data.contacts as { edges: { node: ContactNode }[]; pageInfo: never },
			}),
		{
			wrapper: ({ children }: { children: ReactNode }) => (
				<GraphProvider graph={graph}>{children}</GraphProvider>
			),
		},
	)

	await new Promise((resolve) => setTimeout(resolve, 20))
	expect(served).toEqual([])
	expect(result.current.rows).toEqual([])
	expect(result.current.isPending).toBe(false)
})

test('refreshes from the network when the list is mounted again', async () => {
	let name = 'Ada Lovelace'
	server.use(
		graphql.query('Contacts', () =>
			HttpResponse.json({ data: { contacts: page({ id: 'id-ada', name }, false, 'cursor-ada') } }),
		),
	)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	function Wrapper({ children }: { children: ReactNode }) {
		return <GraphProvider graph={graph}>{children}</GraphProvider>
	}
	const probe = () =>
		useConnection({
			query: contactsQuery,
			variables: { first: 50 },
			select: (data: { contacts: unknown }) =>
				data.contacts as { edges: { node: ContactNode }[]; pageInfo: never },
		})

	const first = renderHook(probe, { wrapper: Wrapper })
	await waitFor(() => expect(first.result.current.rows).toHaveLength(1))
	first.unmount()
	name = 'Ada Lovelace Ltd'
	const second = renderHook(probe, { wrapper: Wrapper })

	await waitFor(() => expect(second.result.current.rows[0]?.name).toBe('Ada Lovelace Ltd'))
})

test('carries the whole document beside the paged rows', async () => {
	servePages({ first: page({ id: 'id-ada', name: 'Ada Lovelace' }, false, 'cursor-ada') })

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.rows).toHaveLength(1))
	const data = result.current.data as { contacts: { edges: unknown[] } } | undefined
	expect(data?.contacts.edges).toHaveLength(1)
})

test('stays on the first page when asked for more before any page arrives', async () => {
	const served = servePages({ first: page({ id: 'id-ada', name: 'Ada Lovelace' }, true, 'cursor-ada') })
	const { result } = renderConnection({ first: 50 })

	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.rows).toHaveLength(1))
	expect(served).toEqual([undefined])
})

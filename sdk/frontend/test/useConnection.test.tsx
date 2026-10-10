import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { gql } from 'urql'
import { expect, test, vi } from 'vitest'

import { GraphProvider } from '../GraphProvider'
import { createGraphClient } from '../graph'
import type { GraphClient } from '../graph'
import { HttpResponse, delay, graphql, server } from '../testing'
import { useConnection } from '../useConnection'

const tasksQuery = gql`
	query Tasks($status: String, $first: Int, $after: String) {
		tasks(date: "2026-10-10", status: $status, first: $first, after: $after) {
			edges {
				node {
					id
					title
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

interface TaskNode {
	id: string
	title: string
}

/** Returns the first rows of a list as one connection the graph serializes. */
function windowOf(rows: TaskNode[], first: number) {
	const shown = rows.slice(0, first)
	return {
		__typename: 'TaskConnection',
		edges: shown.map((row) => ({
			__typename: 'TaskEdge',
			node: { __typename: 'Task', ...row },
			cursor: row.id,
		})),
		pageInfo: { __typename: 'PageInfo', hasNextPage: rows.length > first, endCursor: shown.at(-1)?.id ?? null },
	}
}

/** Answers the tasks query with as many of the listed rows as it asks for, recording each size. */
function serveWindow(list: () => TaskNode[]) {
	const asked: number[] = []
	server.use(
		graphql.query('Tasks', ({ variables }) => {
			const first = Number(variables.first)
			asked.push(first)
			return HttpResponse.json({ data: { tasks: windowOf(list(), first) } })
		}),
	)
	return asked
}

/** Lists the given titles as task rows. */
function rowsOf(...titles: string[]): TaskNode[] {
	return titles.map((title, at) => ({ id: `id-${at}-${title}`, title }))
}

/** Renders the hook inside the given graph client. */
function renderConnection(
	variables: Record<string, unknown>,
	options: { ceiling?: number; graph?: GraphClient } = {},
) {
	const graph = options.graph ?? createGraphClient({ onSessionExpired: vi.fn() })
	function Wrapper({ children }: { children: ReactNode }) {
		return <GraphProvider graph={graph}>{children}</GraphProvider>
	}
	return renderHook(
		(props: { variables: Record<string, unknown> }) =>
			useConnection({
				query: tasksQuery,
				variables: props.variables as { first: number },
				ceiling: options.ceiling,
				select: (data: { tasks: unknown }) => data.tasks as { edges: { node: TaskNode }[]; pageInfo: never },
			}),
		{ wrapper: Wrapper, initialProps: { variables } },
	)
}

/** Returns the titles the hook exposes. */
function titles(rows: TaskNode[]) {
	return rows.map((row) => row.title)
}

test('exposes the rows of the first window', async () => {
	serveWindow(() => rowsOf('Call the supplier'))

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.isPending).toBe(false))
	expect(titles(result.current.rows)).toEqual(['Call the supplier'])
	expect(result.current.hasNextPage).toBe(false)
	expect(result.current.isError).toBe(false)
})

test('grows the window by one page and stops offering more at the end', async () => {
	const asked = serveWindow(() => rowsOf('Call the supplier', 'Send the quote', 'Book the courier'))

	const { result } = renderConnection({ first: 2 })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.rows).toHaveLength(3))
	expect(titles(result.current.rows)).toEqual(['Call the supplier', 'Send the quote', 'Book the courier'])
	expect(result.current.hasNextPage).toBe(false)
	expect(asked).toEqual([2, 4])
})

test('keeps the loaded rows while a bigger window loads', async () => {
	let slow = false
	const listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier')
	server.use(
		graphql.query('Tasks', async ({ variables }) => {
			if (slow) {
				await delay('infinite')
			}
			return HttpResponse.json({ data: { tasks: windowOf(listed, Number(variables.first)) } })
		}),
	)
	const { result } = renderConnection({ first: 2 })
	await waitFor(() => expect(result.current.rows).toHaveLength(2))

	slow = true
	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.isFetchingNextPage).toBe(true))
	expect(titles(result.current.rows)).toEqual(['Call the supplier', 'Send the quote'])
	expect(result.current.isPending).toBe(false)
})

test('asks for no more than the ceiling and stops offering more there', async () => {
	const asked = serveWindow(() => rowsOf('Call the supplier', 'Send the quote', 'Book the courier', 'File the report'))

	const { result } = renderConnection({ first: 2 }, { ceiling: 3 })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.rows).toHaveLength(3))
	expect(result.current.hasNextPage).toBe(false)
	expect(asked).toEqual([2, 3])
})

test('reads every loaded row again when the list is refetched', async () => {
	let listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier')
	const asked = serveWindow(() => listed)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderConnection({ first: 2 }, { graph })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	listed = [{ id: 'id-raised', title: 'Approve the pricing' }, ...listed]
	graph.refetch(['Tasks'])

	await waitFor(() => expect(result.current.rows).toHaveLength(4))
	expect(titles(result.current.rows)).toEqual([
		'Approve the pricing',
		'Call the supplier',
		'Send the quote',
		'Book the courier',
	])
	expect(asked).toEqual([2, 4, 4])
})

test('reports the failure when the window cannot be loaded', async () => {
	server.use(graphql.query('Tasks', () => HttpResponse.json({ data: null, errors: [{ message: 'boom' }] })))

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.isError).toBe(true))
	expect(result.current.rows).toEqual([])
	expect(result.current.isPending).toBe(false)
})

test('starts from one page again when the variables change', async () => {
	const asked = serveWindow(() => rowsOf('Call the supplier', 'Send the quote', 'Book the courier'))
	const { result, rerender } = renderConnection({ first: 2, status: 'open' })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	rerender({ variables: { first: 2, status: 'done' } })

	await waitFor(() => expect(asked.at(-1)).toBe(2))
	await waitFor(() => expect(result.current.rows).toHaveLength(2))
})

test('runs no query while it is paused', async () => {
	const asked = serveWindow(() => rowsOf('Call the supplier'))
	const graph = createGraphClient({ onSessionExpired: vi.fn() })

	const { result } = renderHook(
		() =>
			useConnection({
				query: tasksQuery,
				variables: { first: 50 },
				pause: true,
				select: (data: { tasks: unknown }) => data.tasks as { edges: { node: TaskNode }[]; pageInfo: never },
			}),
		{
			wrapper: ({ children }: { children: ReactNode }) => <GraphProvider graph={graph}>{children}</GraphProvider>,
		},
	)

	await new Promise((resolve) => setTimeout(resolve, 20))
	expect(asked).toEqual([])
	expect(result.current.rows).toEqual([])
	expect(result.current.isPending).toBe(false)
})

test('refreshes from the network when the list is mounted again', async () => {
	let title = 'Call the supplier'
	serveWindow(() => [{ id: 'id-call', title }])
	const graph = createGraphClient({ onSessionExpired: vi.fn() })

	const first = renderConnection({ first: 50 }, { graph })
	await waitFor(() => expect(first.result.current.rows).toHaveLength(1))
	first.unmount()
	title = 'Call the supplier back'
	const second = renderConnection({ first: 50 }, { graph })

	await waitFor(() => expect(second.result.current.rows[0]?.title).toBe('Call the supplier back'))
})

test('carries the whole document beside the rows', async () => {
	serveWindow(() => rowsOf('Call the supplier'))

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.rows).toHaveLength(1))
	const data = result.current.data as { tasks: { edges: unknown[] } } | undefined
	expect(data?.tasks.edges).toHaveLength(1)
})

test('stays on the first window when asked for more before any rows arrive', async () => {
	const asked = serveWindow(() => rowsOf('Call the supplier', 'Send the quote'))
	const { result } = renderConnection({ first: 1 })

	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.rows).toHaveLength(1))
	expect(asked).toEqual([1])
})

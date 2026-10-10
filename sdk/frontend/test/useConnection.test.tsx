// SPDX-License-Identifier: AGPL-3.0-or-later

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

/** Returns one page of task rows as the graph serializes it. */
function pageOf(rows: TaskNode[], hasNextPage: boolean) {
	return {
		__typename: 'TaskConnection',
		edges: rows.map((row) => ({ __typename: 'TaskEdge', node: { __typename: 'Task', ...row }, cursor: row.id })),
		pageInfo: { __typename: 'PageInfo', hasNextPage, endCursor: rows.at(-1)?.id ?? null },
	}
}

/** Answers the tasks query a page at a time after the cursor it names, recording each cursor. */
function servePaged(list: () => TaskNode[]) {
	const asked: (string | null)[] = []
	server.use(
		graphql.query('Tasks', ({ variables }) => {
			const after = (variables.after as string | null | undefined) ?? null
			asked.push(after)
			const rows = list()
			const start = after === null ? 0 : rows.findIndex((row) => row.id === after) + 1
			const end = start + Number(variables.first)
			return HttpResponse.json({ data: { tasks: pageOf(rows.slice(start, end), end < rows.length) } })
		}),
	)
	return asked
}

/** Lists the given titles as task rows keyed by their first word. */
function rowsOf(...titles: string[]): TaskNode[] {
	return titles.map((title) => ({ id: title.split(' ')[0].toLowerCase(), title }))
}

/** Renders the hook inside the given graph client. */
function renderConnection(variables: Record<string, unknown>, graph?: GraphClient) {
	const served = graph ?? createGraphClient({ onSessionExpired: vi.fn() })
	function Wrapper({ children }: { children: ReactNode }) {
		return <GraphProvider graph={served}>{children}</GraphProvider>
	}
	return renderHook(
		(props: { variables: Record<string, unknown> }) =>
			useConnection({
				query: tasksQuery,
				variables: props.variables,
				select: (data: { tasks: unknown }) => data.tasks as { edges: { node: TaskNode }[]; pageInfo: never },
			}),
		{ wrapper: Wrapper, initialProps: { variables } },
	)
}

/** Returns the titles the hook exposes. */
function titles(rows: TaskNode[]) {
	return rows.map((row) => row.title)
}

test('exposes the rows of the first page', async () => {
	servePaged(() => rowsOf('Call the supplier'))

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.isPending).toBe(false))
	expect(titles(result.current.rows)).toEqual(['Call the supplier'])
	expect(result.current.hasNextPage).toBe(false)
	expect(result.current.isError).toBe(false)
})

test('appends the next page through the cursor and stops offering more at the end', async () => {
	const asked = servePaged(() => rowsOf('Call the supplier', 'Send the quote', 'Book the courier'))

	const { result } = renderConnection({ first: 2 })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.rows).toHaveLength(3))
	expect(titles(result.current.rows)).toEqual(['Call the supplier', 'Send the quote', 'Book the courier'])
	expect(result.current.hasNextPage).toBe(false)
	expect(asked).toEqual([null, 'send'])
})

test('reads every loaded page again in order when its list is refetched', async () => {
	let listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier')
	servePaged(() => listed)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderConnection({ first: 2 }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	listed = [...rowsOf('Approve the pricing'), ...listed]
	graph.refetch(['Tasks'])

	await waitFor(() =>
		expect(titles(result.current.rows)).toEqual([
			'Approve the pricing',
			'Call the supplier',
			'Send the quote',
			'Book the courier',
		]),
	)
	expect(result.current.hasNextPage).toBe(false)
})

test('keeps offering more after a reread when the list holds further pages', async () => {
	const listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier', 'File the report', 'Chase it')
	const asked = servePaged(() => listed)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderConnection({ first: 2 }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(4))

	graph.refetch(['Tasks'])

	await waitFor(() => expect(asked.filter((after) => after === 'send')).toHaveLength(2))
	expect(titles(result.current.rows)).toHaveLength(4)
	expect(result.current.hasNextPage).toBe(true)
})

test('keeps the loaded pages and reports the failure when a reread fails', async () => {
	let failing = false
	const listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier')
	server.use(
		graphql.query('Tasks', ({ variables }) => {
			if (failing && variables.after) {
				return HttpResponse.json({ data: null, errors: [{ message: 'boom' }] })
			}
			const start = variables.after ? listed.findIndex((row) => row.id === variables.after) + 1 : 0
			return HttpResponse.json({ data: { tasks: pageOf(listed.slice(start, start + 2), start + 2 < listed.length) } })
		}),
	)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderConnection({ first: 2 }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	failing = true
	graph.refetch(['Tasks'])

	await waitFor(() => expect(result.current.isError).toBe(true))
	expect(titles(result.current.rows)).toEqual(['Call the supplier', 'Send the quote', 'Book the courier'])
})

test('keeps the newest reread when an older one lands last', async () => {
	let listed = rowsOf('Approve the pricing', 'Call the supplier', 'Send the quote', 'Book the courier')
	let hold = false
	let release = () => {}
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		graphql.query('Tasks', async ({ variables }) => {
			if (hold && variables.after) {
				hold = false
				await held
			}
			const start = variables.after ? listed.findIndex((row) => row.id === variables.after) + 1 : 0
			return HttpResponse.json({ data: { tasks: pageOf(listed.slice(start, start + 2), start + 2 < listed.length) } })
		}),
	)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderConnection({ first: 2 }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(4))

	hold = true
	graph.refetch(['Tasks'])
	await waitFor(() => expect(hold).toBe(false))
	listed = [...rowsOf('Renew the contract'), ...listed]
	graph.refetch(['Tasks'])
	await waitFor(() => expect(titles(result.current.rows)[0]).toBe('Renew the contract'))
	await waitFor(() => expect(titles(result.current.rows)).toHaveLength(4))
	release()
	await new Promise((resolve) => setTimeout(resolve, 50))

	expect(titles(result.current.rows)).toEqual([
		'Renew the contract',
		'Approve the pricing',
		'Call the supplier',
		'Send the quote',
	])
})

/** Answers the open and done lists a row at a time, holding the later pages of open work until released. */
function serveHeldOpen() {
	let release = () => {}
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	const control = { holding: false, release }
	server.use(
		graphql.query('Tasks', async ({ variables }) => {
			if (control.holding && variables.after && variables.status === 'open') {
				await held
			}
			const listed =
				variables.status === 'done'
					? rowsOf('Book the courier', 'File the receipt')
					: rowsOf('Call the supplier', 'Send the quote')
			const start = variables.after ? listed.findIndex((row) => row.id === variables.after) + 1 : 0
			return HttpResponse.json({ data: { tasks: pageOf(listed.slice(start, start + 1), start + 1 < listed.length) } })
		}),
	)
	return control
}

test('drops a reread that lands after the variables changed', async () => {
	const open = serveHeldOpen()
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result, rerender } = renderConnection({ first: 1, status: 'open' }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(2))

	open.holding = true
	graph.refetch(['Tasks'])
	rerender({ variables: { first: 1, status: 'done' } })
	await waitFor(() => expect(titles(result.current.rows)).toEqual(['Book the courier']))
	void result.current.fetchNextPage()
	await waitFor(() => expect(titles(result.current.rows)).toEqual(['Book the courier', 'File the receipt']))
	open.release()
	await new Promise((resolve) => setTimeout(resolve, 50))

	expect(titles(result.current.rows)).toEqual(['Book the courier', 'File the receipt'])
})

test('drops a next page that lands after the variables changed', async () => {
	const open = serveHeldOpen()
	open.holding = true
	const { result, rerender } = renderConnection({ first: 1, status: 'open' })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.isFetchingNextPage).toBe(true))

	rerender({ variables: { first: 1, status: 'done' } })
	await waitFor(() => expect(titles(result.current.rows)).toEqual(['Book the courier']))
	void result.current.fetchNextPage()
	await waitFor(() => expect(titles(result.current.rows)).toEqual(['Book the courier', 'File the receipt']))
	open.release()
	await new Promise((resolve) => setTimeout(resolve, 50))

	expect(titles(result.current.rows)).toEqual(['Book the courier', 'File the receipt'])
})

test('lists nothing when the document holds no connection', async () => {
	server.use(graphql.query('Tasks', () => HttpResponse.json({ data: { tasks: null } })))

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.isPending).toBe(false))
	expect(result.current.rows).toEqual([])
	expect(result.current.hasNextPage).toBe(false)
})

test('reads no further page when another list is refetched', async () => {
	const asked = servePaged(() => rowsOf('Call the supplier', 'Send the quote', 'Book the courier'))
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderConnection({ first: 2 }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	graph.refetch(['Contacts'])

	await new Promise((resolve) => setTimeout(resolve, 20))
	expect(asked).toEqual([null, 'send'])
})

test('keeps the loaded rows while the next page loads', async () => {
	const listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier')
	server.use(
		graphql.query('Tasks', async ({ variables }) => {
			if (variables.after) {
				await delay('infinite')
			}
			return HttpResponse.json({ data: { tasks: pageOf(listed.slice(0, 2), true) } })
		}),
	)
	const { result } = renderConnection({ first: 2 })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))

	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.isFetchingNextPage).toBe(true))
	expect(titles(result.current.rows)).toEqual(['Call the supplier', 'Send the quote'])
	expect(result.current.isPending).toBe(false)
})

test('lists a row once when two pages both carry it', async () => {
	server.use(
		graphql.query('Tasks', ({ variables }) =>
			HttpResponse.json({
				data: {
					tasks: variables.after
						? pageOf(rowsOf('Send the quote', 'Book the courier'), false)
						: pageOf(rowsOf('Call the supplier', 'Send the quote'), true),
				},
			}),
		),
	)
	const { result } = renderConnection({ first: 2 })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))

	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.hasNextPage).toBe(false))
	expect(titles(result.current.rows)).toEqual(['Call the supplier', 'Send the quote', 'Book the courier'])
})

test('reports the failure when the first page cannot be loaded', async () => {
	server.use(graphql.query('Tasks', () => HttpResponse.json({ data: null, errors: [{ message: 'boom' }] })))

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.isError).toBe(true))
	expect(result.current.rows).toEqual([])
	expect(result.current.isPending).toBe(false)
})

test('reports the failure when the next page cannot be loaded and keeps the rows', async () => {
	server.use(
		graphql.query('Tasks', ({ variables }) =>
			variables.after
				? HttpResponse.json({ data: null, errors: [{ message: 'boom' }] })
				: HttpResponse.json({ data: { tasks: pageOf(rowsOf('Call the supplier'), true) } }),
		),
	)
	const { result } = renderConnection({ first: 1 })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))

	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.isError).toBe(true))
	expect(titles(result.current.rows)).toEqual(['Call the supplier'])
})

test('starts from the first page again when the variables change', async () => {
	const asked = servePaged(() => rowsOf('Call the supplier', 'Send the quote', 'Book the courier'))
	const { result, rerender } = renderConnection({ first: 2, status: 'open' })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	rerender({ variables: { first: 2, status: 'done' } })

	await waitFor(() => expect(result.current.rows).toHaveLength(2))
	expect(asked.at(-1)).toBeNull()
})

test('runs no query while it is paused', async () => {
	const asked = servePaged(() => rowsOf('Call the supplier'))
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
	servePaged(() => [{ id: 'call', title }])
	const graph = createGraphClient({ onSessionExpired: vi.fn() })

	const first = renderConnection({ first: 50 }, graph)
	await waitFor(() => expect(first.result.current.rows).toHaveLength(1))
	first.unmount()
	title = 'Call the supplier back'
	const second = renderConnection({ first: 50 }, graph)

	await waitFor(() => expect(second.result.current.rows[0]?.title).toBe('Call the supplier back'))
})

test('carries the whole document beside the rows', async () => {
	servePaged(() => rowsOf('Call the supplier'))

	const { result } = renderConnection({ first: 50 })

	await waitFor(() => expect(result.current.rows).toHaveLength(1))
	const data = result.current.data as { tasks: { edges: unknown[] } } | undefined
	expect(data?.tasks.edges).toHaveLength(1)
})

test('stays on the first page when asked for more before any rows arrive', async () => {
	const asked = servePaged(() => rowsOf('Call the supplier', 'Send the quote'))
	const { result } = renderConnection({ first: 1 })

	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.rows).toHaveLength(1))
	expect(asked).toEqual([null])
})

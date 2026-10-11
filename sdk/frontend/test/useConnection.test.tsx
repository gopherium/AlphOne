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

/** Answers the open and done lists a row at a time, holding each list's later pages until that list is released. */
function serveHeldLaterPages() {
	const releases = { open: () => {}, done: () => {} }
	const held = {
		open: new Promise<void>((resolve) => {
			releases.open = resolve
		}),
		done: new Promise<void>((resolve) => {
			releases.done = resolve
		}),
	}
	server.use(
		graphql.query('Tasks', async ({ variables }) => {
			const status = variables.status === 'done' ? 'done' : 'open'
			if (variables.after) {
				await held[status]
			}
			const listed =
				status === 'done' ? rowsOf('Book the courier', 'File the receipt') : rowsOf('Call the supplier', 'Send the quote')
			const start = variables.after ? listed.findIndex((row) => row.id === variables.after) + 1 : 0
			return HttpResponse.json({ data: { tasks: pageOf(listed.slice(start, start + 1), start + 1 < listed.length) } })
		}),
	)
	return releases
}

test('keeps the next page of the new list loading when a next page of the old list lands', async () => {
	const release = serveHeldLaterPages()
	const { result, rerender } = renderConnection({ first: 1, status: 'open' })
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.isFetchingNextPage).toBe(true))

	rerender({ variables: { first: 1, status: 'done' } })
	await waitFor(() => expect(titles(result.current.rows)).toEqual(['Book the courier']))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.isFetchingNextPage).toBe(true))
	release.open()
	await settle()

	expect(result.current.isFetchingNextPage).toBe(true)
	release.done()
	await waitFor(() => expect(titles(result.current.rows)).toEqual(['Book the courier', 'File the receipt']))
	expect(result.current.isFetchingNextPage).toBe(false)
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

/** HeldRead names one read the held server answered. */
interface HeldRead {
	status: string | null
	after: string | null
}

/**
 * Answers each status's list a page at a time after the cursor row, holding the first read a test names.
 * @param lists - The rows of each status.
 * @param keyset - Whether a cursor names a position, as the server's keyset does, rather than a row.
 * @returns The reads so far beside the hold and its release.
 */
function serveHeld(lists: (status: string | null) => TaskNode[], keyset = false) {
	let release = () => {}
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	const gate = { reads: [] as HeldRead[], holdIf: (_read: HeldRead) => false, release: () => release() }
	server.use(
		graphql.query('Tasks', async ({ variables }) => {
			const read: HeldRead = {
				status: (variables.status as string | undefined) ?? null,
				after: (variables.after as string | null | undefined) ?? null,
			}
			gate.reads.push(read)
			if (gate.holdIf(read)) {
				gate.holdIf = () => false
				await held
			}
			const rows = lists(read.status)
			const start = startOf(rows, read.after, keyset)
			const end = start + Number(variables.first)
			return HttpResponse.json({ data: { tasks: pageOf(rows.slice(start, end), end < rows.length) } })
		}),
	)
	return gate
}

/** Returns where a page after the cursor starts in the rows. */
function startOf(rows: TaskNode[], after: string | null, keyset: boolean) {
	if (after === null) {
		return 0
	}
	const at = keyset ? rows.findIndex((row) => row.id > after) : rows.findIndex((row) => row.id === after) + 1
	return at < 0 ? rows.length : at
}

/** Renders the hook with variables and a pause flag a test can change. */
function renderSwitchable(initial: { variables: Record<string, unknown>; pause?: boolean }, graph: GraphClient) {
	function Wrapper({ children }: { children: ReactNode }) {
		return <GraphProvider graph={graph}>{children}</GraphProvider>
	}
	return renderHook(
		(props: { variables: Record<string, unknown>; pause?: boolean }) =>
			useConnection({
				query: tasksQuery,
				variables: props.variables,
				pause: props.pause,
				select: (data: { tasks: unknown }) => data.tasks as { edges: { node: TaskNode }[]; pageInfo: never },
			}),
		{ wrapper: Wrapper, initialProps: initial },
	)
}

/** Waits long enough for reads already answered to land. */
function settle() {
	return new Promise((resolve) => setTimeout(resolve, 80))
}

test('keeps a page loaded while a reread runs listed after the reread lands', async () => {
	const listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier', 'File the report', 'Chase it')
	const gate = serveHeld(() => listed)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderSwitchable({ variables: { first: 2 } }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(4))

	gate.holdIf = (read) => read.after === 'send'
	graph.refetch(['Tasks'])
	await waitFor(() => expect(gate.reads.filter((read) => read.after === 'send')).toHaveLength(2))
	void result.current.fetchNextPage()
	await settle()
	gate.release()

	await waitFor(() => expect(titles(result.current.rows)).toEqual(titles(listed)))
	await settle()
	expect(titles(result.current.rows)).toEqual(titles(listed))
	expect(result.current.hasNextPage).toBe(false)
})

test('leaves no row out when a reread lands while the next page is in flight', async () => {
	let listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier', 'File the report', 'Chase it', 'Pay it')
	const gate = serveHeld(() => listed)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderSwitchable({ variables: { first: 2 } }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(4))

	listed = [...rowsOf('Approve the pricing'), ...listed]
	gate.holdIf = (read) => read.after === 'file'
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.isFetchingNextPage).toBe(true))
	graph.refetch(['Tasks'])
	await settle()
	gate.release()
	await settle()

	const shown = titles(result.current.rows)
	expect(shown).toEqual(titles(listed).slice(0, shown.length))
	expect(result.current.hasNextPage || shown.length === listed.length).toBe(true)
})

test('reads no cursor of the old list when load more is pressed right after the variables change', async () => {
	const done = rowsOf('A1 done', 'A2 done', 'A3 done', 'Z1 done')
	const gate = serveHeld((status) => (status === 'done' ? done : rowsOf('M1 open', 'M2 open', 'M3 open')), true)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result, rerender } = renderSwitchable({ variables: { first: 2, status: 'open' } }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))

	gate.holdIf = (read) => read.status === 'done' && read.after === null
	rerender({ variables: { first: 2, status: 'done' } })
	await waitFor(() => expect(gate.reads.some((read) => read.status === 'done')).toBe(true))
	void result.current.fetchNextPage()
	await settle()
	gate.release()
	await settle()

	expect(gate.reads.filter((read) => read.status === 'done').map((read) => read.after)).not.toContain('m2')
	const shown = titles(result.current.rows)
	expect(shown).toEqual(['A1 done', 'A2 done', 'A3 done', 'Z1 done'].slice(0, shown.length))
})

test('reads a list from its first page again when the variables return to it', async () => {
	let open = rowsOf('Call the supplier', 'Send the quote', 'Book the courier')
	serveHeld((status) => (status === 'done' ? rowsOf('Pay the invoice') : open))
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result, rerender } = renderSwitchable({ variables: { first: 2, status: 'open' } }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	rerender({ variables: { first: 2, status: 'done' } })
	await waitFor(() => expect(titles(result.current.rows)).toEqual(['Pay the invoice']))
	open = rowsOf('Call the supplier', 'Send the quote')
	graph.refetch(['Tasks'])
	await settle()
	rerender({ variables: { first: 2, status: 'open' } })
	await settle()

	expect(titles(result.current.rows)).toEqual(['Call the supplier', 'Send the quote'])
})

test('reads nothing while paused when its list is refetched', async () => {
	const gate = serveHeld(() => rowsOf('Call the supplier', 'Send the quote', 'Book the courier'))
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result, rerender } = renderSwitchable({ variables: { first: 2 }, pause: false }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	rerender({ variables: { first: 2 }, pause: true })
	await settle()
	const before = gate.reads.length
	graph.refetch(['Tasks'])
	graph.refetch(['Tasks'])
	await settle()

	expect(gate.reads.slice(before)).toEqual([])
})

test('reads its loaded pages again once it resumes after its list was refetched while paused', async () => {
	let listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier')
	serveHeld(() => listed)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result, rerender } = renderSwitchable({ variables: { first: 2 }, pause: false }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	rerender({ variables: { first: 2 }, pause: true })
	await settle()
	listed = rowsOf('Call the supplier', 'Send the quote', 'File the report')
	graph.refetch(['Tasks'])
	await settle()
	rerender({ variables: { first: 2 }, pause: false })

	await waitFor(() =>
		expect(titles(result.current.rows)).toEqual(['Call the supplier', 'Send the quote', 'File the report']),
	)
})

test('reads nothing on resume when its list was not refetched while paused', async () => {
	const gate = serveHeld(() => rowsOf('Call the supplier', 'Send the quote', 'Book the courier'))
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result, rerender } = renderSwitchable({ variables: { first: 2 }, pause: false }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	rerender({ variables: { first: 2 }, pause: true })
	await settle()
	const before = gate.reads.length
	rerender({ variables: { first: 2 }, pause: false })
	await settle()

	expect(gate.reads.slice(before).filter((read) => read.after !== null)).toEqual([])
})

test('settles a burst of refetches on the newest list with one walk at a time', async () => {
	let listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier', 'File the report')
	const gate = serveHeld(() => listed)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderSwitchable({ variables: { first: 2 } }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(4))
	const before = gate.reads.length

	for (const title of ['Approve the pricing', 'Renew the contract', 'Quote the job']) {
		listed = [...rowsOf(title), ...listed]
		graph.refetch(['Tasks'])
	}
	await settle()
	await settle()

	expect(titles(result.current.rows)).toEqual(titles(listed).slice(0, 4))
	expect(gate.reads.slice(before).filter((read) => read.after !== null).length).toBeLessThanOrEqual(2)
})

test('clears a failed load more once its list is refetched', async () => {
	let failing = true
	server.use(
		graphql.query('Tasks', ({ variables }) =>
			failing && variables.after
				? HttpResponse.json({ data: null, errors: [{ message: 'boom' }] })
				: HttpResponse.json({ data: { tasks: pageOf(rowsOf('Call the supplier'), true) } }),
		),
	)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderConnection({ first: 1 }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.isError).toBe(true))

	failing = false
	graph.refetch(['Tasks'])

	await waitFor(() => expect(result.current.isError).toBe(false))
})

test('clears a failed reread once a later reread succeeds', async () => {
	let failing = false
	const listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier')
	server.use(
		graphql.query('Tasks', ({ variables }) => {
			if (failing && variables.after) {
				return HttpResponse.json({ data: null, errors: [{ message: 'boom' }] })
			}
			const start = startOf(listed, (variables.after as string | null | undefined) ?? null, false)
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

	failing = false
	graph.refetch(['Tasks'])

	await waitFor(() => expect(result.current.isError).toBe(false))
	expect(titles(result.current.rows)).toEqual(titles(listed))
})

test('lists fewer pages after a reread when work left the list', async () => {
	let listed = rowsOf('Call the supplier', 'Send the quote', 'Book the courier')
	serveHeld(() => listed)
	const graph = createGraphClient({ onSessionExpired: vi.fn() })
	const { result } = renderSwitchable({ variables: { first: 2 } }, graph)
	await waitFor(() => expect(result.current.hasNextPage).toBe(true))
	void result.current.fetchNextPage()
	await waitFor(() => expect(result.current.rows).toHaveLength(3))

	listed = rowsOf('Call the supplier', 'Send the quote')
	graph.refetch(['Tasks'])

	await waitFor(() => expect(titles(result.current.rows)).toEqual(['Call the supplier', 'Send the quote']))
	expect(result.current.hasNextPage).toBe(false)
})

test('stays on the first page when asked for more before any rows arrive', async () => {
	const asked = servePaged(() => rowsOf('Call the supplier', 'Send the quote'))
	const { result } = renderConnection({ first: 1 })

	void result.current.fetchNextPage()

	await waitFor(() => expect(result.current.rows).toHaveLength(1))
	expect(asked).toEqual([null])
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { useEffect, useRef, useState } from 'react'
import { getOperationName, useQuery } from 'urql'
import type { AnyVariables, Client, TypedDocumentNode } from 'urql'

import { useGraph } from './GraphProvider'

/** ConnectionPage is one Relay page as the graph serializes it. */
export interface ConnectionPage<TNode> {
	edges: { node: TNode }[]
	pageInfo: { hasNextPage: boolean; endCursor?: string | null }
}

/** ConnectionResult carries the merged rows beside the load more controls. */
export interface ConnectionResult<TNode, TData = unknown> {
	/** rows are every node loaded so far, first page first, each listed once. */
	rows: TNode[]
	/** data is the whole document the first page was selected from. */
	data: TData | undefined
	/** isPending reports that the first page has not arrived. */
	isPending: boolean
	/** isError reports that a page could not be loaded. */
	isError: boolean
	/** hasNextPage reports that another page is available. */
	hasNextPage: boolean
	/** isFetchingNextPage reports that a further page is in flight. */
	isFetchingNextPage: boolean
	/** fetchNextPage advances to the next page. */
	fetchNextPage: () => Promise<void>
}

/** ConnectionOptions names the document a connection is read from and how its page is found. */
interface ConnectionOptions<TData, TVariables extends AnyVariables, TNode> {
	query: TypedDocumentNode<TData, TVariables>
	variables: TVariables & { after?: string | null }
	select: (data: TData) => ConnectionPage<TNode> | null | undefined
	pause?: boolean
}

/** laterPages carries the pages loaded past the first beside the variables they belong to. */
interface laterPages<TNode> {
	key: string
	pages: ConnectionPage<TNode>[]
	fetching: boolean
	failed: boolean
}

/**
 * Pages one Relay connection by cursor, reading every loaded page again in order whenever its list is refetched.
 * @param options - The document, its variables, the page selector, and the pause flag.
 * @returns The merged rows beside the load more controls.
 */
export function useConnection<TData, TVariables extends AnyVariables, TNode>(
	options: ConnectionOptions<TData, TVariables, TNode>,
): ConnectionResult<TNode, TData> {
	const key = JSON.stringify(options.variables)
	const graph = useGraph()
	const [result] = useQuery<TData, TVariables>({
		query: options.query,
		variables: { ...options.variables, after: undefined },
		pause: options.pause,
		requestPolicy: 'cache-and-network',
	})
	const head = pageIn(result.data, options.select)
	const [later, setLater] = useState<laterPages<TNode>>(() => noLaterPages(key))
	const current = later.key === key ? later : noLaterPages<TNode>(key)
	useRereadOnRefetch(graph, options, key, current.pages.length, setLater)
	const last = current.pages.at(-1) ?? head
	return {
		rows: rowsOf([head, ...current.pages]),
		data: result.data,
		isPending: result.fetching && head === undefined,
		isError: result.error !== undefined || current.failed,
		hasNextPage: last?.pageInfo.hasNextPage ?? false,
		isFetchingNextPage: current.fetching,
		fetchNextPage: async () => {
			if (!last?.pageInfo.hasNextPage || current.fetching) {
				return
			}
			setLater({ ...current, fetching: true })
			const page = await readPage(graph.client, options, last.pageInfo.endCursor)
			setLater((now) => (now.key === key ? appended(now, page) : now))
		},
	}
}

/**
 * Reads every loaded page again from the first whenever the doorbell names this connection's document.
 * @param graph - The graph client whose doorbell is heard.
 * @param options - The connection's document, variables and page selector.
 * @param key - The variables the loaded pages belong to.
 * @param count - How many pages past the first are loaded.
 * @param setLater - The setter of the pages past the first.
 */
function useRereadOnRefetch<TData, TVariables extends AnyVariables, TNode>(
	graph: { client: Client; onRefetch: (listener: (operations: readonly string[]) => void) => () => void },
	options: ConnectionOptions<TData, TVariables, TNode>,
	key: string,
	count: number,
	setLater: (update: (now: laterPages<TNode>) => laterPages<TNode>) => void,
) {
	const latest = useRef({ options, key, count })
	const generation = useRef(0)
	useEffect(() => {
		latest.current = { options, key, count }
	})
	const name = getOperationName(options.query)
	useEffect(
		() =>
			graph.onRefetch((operations) => {
				const held = latest.current
				if (held.count === 0 || name === undefined || !operations.includes(name)) {
					return
				}
				generation.current += 1
				const walk = generation.current
				void rereadPages(graph.client, held.options, held.count).then((pages) => {
					if (walk === generation.current) {
						setLater((now) => (now.key === held.key ? reread(now, pages) : now))
					}
				})
			}),
		[graph, name, setLater],
	)
}

/**
 * Reads the first page and up to count pages after it, each from the cursor the one before it ends on.
 * @param client - The graph client reading the pages.
 * @param options - The connection's document, variables and page selector.
 * @param count - How many pages past the first to read.
 * @returns The pages past the first, as many as the list still holds, or undefined when a read failed.
 */
async function rereadPages<TData, TVariables extends AnyVariables, TNode>(
	client: Client,
	options: ConnectionOptions<TData, TVariables, TNode>,
	count: number,
): Promise<ConnectionPage<TNode>[] | undefined> {
	const pages: ConnectionPage<TNode>[] = []
	let after: string | null | undefined
	do {
		const page = await readPage(client, options, after)
		if (page === undefined) {
			return undefined
		}
		pages.push(page)
		after = page.pageInfo.endCursor
		if (!page.pageInfo.hasNextPage) {
			break
		}
	} while (pages.length <= count)
	return pages.slice(1)
}

/**
 * Reads one page after the given cursor from the network.
 * @param client - The graph client reading the page.
 * @param options - The connection's document, variables and page selector.
 * @param after - The cursor the page starts after, none for the first page.
 * @returns The page, or undefined when it could not be read.
 */
async function readPage<TData, TVariables extends AnyVariables, TNode>(
	client: Client,
	options: ConnectionOptions<TData, TVariables, TNode>,
	after: string | null | undefined,
): Promise<ConnectionPage<TNode> | undefined> {
	const variables = { ...options.variables, after } as TVariables
	const answer = await client.query(options.query, variables, { requestPolicy: 'network-only' }).toPromise()
	return pageIn(answer.data, options.select)
}

/**
 * Returns the page a document carries, undefined when the document or its connection is missing.
 * @param data - The document, if one arrived.
 * @param select - The page selector.
 * @returns The page.
 */
function pageIn<TData, TNode>(
	data: TData | null | undefined,
	select: (data: TData) => ConnectionPage<TNode> | null | undefined,
): ConnectionPage<TNode> | undefined {
	return data ? (select(data) ?? undefined) : undefined
}

/**
 * Returns the pages past the first after a reread, kept as they were and marked failed when it could not be read.
 * @param now - The pages loaded so far.
 * @param pages - The pages read again, undefined when a read failed.
 * @returns The updated pages.
 */
function reread<TNode>(now: laterPages<TNode>, pages: ConnectionPage<TNode>[] | undefined): laterPages<TNode> {
	if (pages === undefined) {
		return { ...now, failed: true }
	}
	return { ...now, pages, failed: false }
}

/**
 * Returns the pages past the first with one more page appended, or marked failed when it could not be read.
 * @param now - The pages loaded so far.
 * @param page - The page read, undefined when the read failed.
 * @returns The updated pages.
 */
function appended<TNode>(now: laterPages<TNode>, page: ConnectionPage<TNode> | undefined): laterPages<TNode> {
	if (page === undefined) {
		return { ...now, fetching: false, failed: true }
	}
	return { ...now, pages: [...now.pages, page], fetching: false, failed: false }
}

/**
 * Returns the rows of the given pages in order, each node with an id listed once.
 * @param pages - The pages, the first possibly not loaded yet.
 * @returns The rows.
 */
function rowsOf<TNode>(pages: (ConnectionPage<TNode> | undefined)[]): TNode[] {
	const seen = new Set<unknown>()
	const rows: TNode[] = []
	for (const edge of pages.flatMap((page) => page?.edges ?? [])) {
		const id = (edge.node as { id?: unknown }).id
		if (id === undefined || !seen.has(id)) {
			seen.add(id)
			rows.push(edge.node)
		}
	}
	return rows
}

/**
 * Returns the state of a connection holding no page past the first.
 * @param key - The variables the state belongs to.
 * @returns The empty state.
 */
function noLaterPages<TNode>(key: string): laterPages<TNode> {
	return { key, pages: [], fetching: false, failed: false }
}

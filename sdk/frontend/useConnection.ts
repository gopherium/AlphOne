// SPDX-License-Identifier: AGPL-3.0-or-later

import { useEffect, useRef, useState } from 'react'
import { getOperationName, useQuery } from 'urql'
import type { AnyVariables, Client, TypedDocumentNode, UseQueryState } from 'urql'

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

/** pageLoader reads one connection's pages past the first one job at a time, a burst of rereads run as one. */
interface pageLoader<TNode> extends laterPages<TNode> {
	generation: number
	running: boolean
	wantMore: boolean
	wantReread: boolean
}

/** loaderInput is what a loader reads with: the graph client, the options, the first page and the pause flag. */
interface loaderInput<TData, TVariables extends AnyVariables, TNode> {
	client: Client
	options: ConnectionOptions<TData, TVariables, TNode>
	head: ConnectionPage<TNode> | undefined
	paused: boolean
}

/** loadedRows is what a reread reads again: the page count past the first, every loaded id and the last page's ids. */
interface loadedRows {
	count: number
	ids: Set<unknown>
	last: Set<unknown>
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
	const variables = { ...options.variables, after: undefined }
	const [result] = useQuery<TData, TVariables>({
		query: options.query,
		variables,
		pause: options.pause,
		requestPolicy: 'cache-and-network',
	})
	const shown = pageIn(result.data, options.select)
	const head = answers(result, variables) ? shown : undefined
	const [later, setLater] = useState<laterPages<TNode>>(() => noLaterPages(key))
	const paused = options.pause === true
	const input = useRef<loaderInput<TData, TVariables, TNode>>({ client: graph.client, options, head, paused })
	useEffect(() => {
		input.current = { client: graph.client, options, head, paused }
	})
	const loader = usePageLoader<TData, TVariables, TNode>(graph, options.query, key, paused, input, setLater)
	const current = later.key === key ? later : noLaterPages<TNode>(key)
	const last = current.pages.at(-1) ?? head
	return {
		rows: rowsOf([shown, ...current.pages]),
		data: result.data,
		isPending: result.fetching && shown === undefined,
		isError: result.error !== undefined || current.failed,
		hasNextPage: last?.pageInfo.hasNextPage === true,
		isFetchingNextPage: current.fetching,
		fetchNextPage: async () => {
			loader.wantMore = true
			await drain(loader, input, () => setLater(snapshot(loader)))
		},
	}
}

/**
 * Keeps one loader per connection, reset on new variables, rereading after each refetch of its list once unpaused.
 * @param graph - The graph client whose doorbell is heard.
 * @param query - The connection's document.
 * @param key - The variables the loader's pages belong to.
 * @param paused - Whether the connection is paused.
 * @param input - What the loader reads with, kept current every render.
 * @param setLater - The setter of the pages past the first.
 * @returns The loader.
 */
function usePageLoader<TData, TVariables extends AnyVariables, TNode>(
	graph: { onRefetch: (listener: (operations: readonly string[]) => void) => () => void },
	query: TypedDocumentNode<TData, TVariables>,
	key: string,
	paused: boolean,
	input: { current: loaderInput<TData, TVariables, TNode> },
	setLater: (later: laterPages<TNode>) => void,
): pageLoader<TNode> {
	const [loader] = useState<pageLoader<TNode>>(() => newLoader(key))
	useEffect(() => {
		if (loader.key !== key) {
			reset(loader, key)
			setLater(snapshot(loader))
		}
	}, [loader, key, setLater])
	const name = getOperationName(query)
	useEffect(
		() =>
			graph.onRefetch((operations) => {
				if (name !== undefined && operations.includes(name)) {
					loader.wantReread = true
					if (!input.current.paused) {
						void drain(loader, input, () => setLater(snapshot(loader)))
					}
				}
			}),
		[graph, name, loader, input, setLater],
	)
	useEffect(() => {
		if (!paused && loader.wantReread) {
			void drain(loader, input, () => setLater(snapshot(loader)))
		}
	}, [paused, loader, input, setLater])
	return loader
}

/**
 * Runs the loader's waiting jobs one at a time, a reread first, until none waits or a reread meets a pause.
 * @param loader - The loader.
 * @param input - What the loader reads with.
 * @param publish - Shows the loader's pages to the screen.
 */
async function drain<TData, TVariables extends AnyVariables, TNode>(
	loader: pageLoader<TNode>,
	input: { current: loaderInput<TData, TVariables, TNode> },
	publish: () => void,
) {
	if (loader.running) {
		return
	}
	loader.running = true
	const generation = loader.generation
	while (generation === loader.generation && (loader.wantReread || loader.wantMore)) {
		if (loader.wantReread && input.current.paused) {
			break
		}
		if (loader.wantReread) {
			loader.wantReread = false
			await reread(loader, input.current)
		} else {
			loader.wantMore = false
			await readMore(loader, input, publish)
		}
		publish()
	}
	if (generation === loader.generation) {
		loader.running = false
	}
}

/**
 * Reads every loaded page again from the first, keeping the pages and marking the failure when a read fails.
 * @param loader - The loader.
 * @param input - What the loader reads with.
 */
async function reread<TData, TVariables extends AnyVariables, TNode>(
	loader: pageLoader<TNode>,
	input: loaderInput<TData, TVariables, TNode>,
) {
	const generation = loader.generation
	if (loader.pages.length === 0) {
		loader.failed = false
		return
	}
	const pages = await rereadPages(input.client, input.options, loadedRowsOf(input.head, loader.pages))
	if (generation === loader.generation) {
		loader.failed = pages === undefined
		loader.pages = pages ?? loader.pages
	}
}

/**
 * Reads the page after the last one loaded, from the first page only once it answers the current variables.
 * @param loader - The loader.
 * @param input - What the loader reads with.
 * @param publish - Shows the loader's pages to the screen.
 */
async function readMore<TData, TVariables extends AnyVariables, TNode>(
	loader: pageLoader<TNode>,
	input: { current: loaderInput<TData, TVariables, TNode> },
	publish: () => void,
) {
	const generation = loader.generation
	const last = loader.pages.at(-1) ?? input.current.head
	if (!last?.pageInfo.hasNextPage) {
		return
	}
	loader.fetching = true
	publish()
	const page = await readPage(input.current.client, input.current.options, last.pageInfo.endCursor)
	if (generation === loader.generation) {
		loader.fetching = false
		loader.failed = page === undefined
		loader.pages = page === undefined ? loader.pages : [...loader.pages, page]
	}
}

/**
 * Reads the first page and the pages after it by cursor, past the loaded count while the pages bring back loaded rows.
 * @param client - The graph client reading the pages.
 * @param options - The connection's document, variables and page selector.
 * @param loaded - The rows loaded before the reread.
 * @returns The pages past the first, or undefined when a read failed.
 */
async function rereadPages<TData, TVariables extends AnyVariables, TNode>(
	client: Client,
	options: ConnectionOptions<TData, TVariables, TNode>,
	loaded: loadedRows,
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
	} while (readsOn(pages, loaded))
	return keptPages(pages.slice(1), loaded)
}

/**
 * Returns the reread pages past the first, without a last page past the loaded count that holds no loaded row.
 * @param later - The pages the reread read past the first.
 * @param loaded - The rows loaded before the reread.
 * @returns The pages the list keeps.
 */
function keptPages<TNode>(later: ConnectionPage<TNode>[], loaded: loadedRows): ConnectionPage<TNode>[] {
	if (later.length <= loaded.count || [...idsOf(later.slice(-1))].some((id) => loaded.ids.has(id))) {
		return later
	}
	return later.slice(0, -1)
}

/**
 * Reports whether a reread reads on, up to the loaded count and past it while loaded rows of the last page are unread.
 * @param pages - The pages the reread has read, the first page first.
 * @param loaded - The rows loaded before the reread.
 * @returns Whether to read on, past the loaded count only while the page just read held a loaded row.
 */
function readsOn<TNode>(pages: ConnectionPage<TNode>[], loaded: loadedRows): boolean {
	if (pages.length <= loaded.count) {
		return true
	}
	const read = idsOf(pages)
	const latest = idsOf(pages.slice(-1))
	return [...loaded.last].some((id) => !read.has(id)) && [...latest].some((id) => loaded.ids.has(id))
}

/**
 * Returns the rows a reread reads again, from the first page and the pages past it.
 * @param head - The first page, if it answers the current variables.
 * @param pages - The pages past the first.
 * @returns The page count beside the ids of every loaded row and of the last page's rows.
 */
function loadedRowsOf<TNode>(head: ConnectionPage<TNode> | undefined, pages: ConnectionPage<TNode>[]): loadedRows {
	return { count: pages.length, ids: idsOf([head, ...pages]), last: idsOf(pages.slice(-1)) }
}

/**
 * Returns the ids of the rows the given pages hold.
 * @param pages - The pages, the first possibly not loaded yet.
 * @returns The ids, rows without one left out.
 */
function idsOf<TNode>(pages: (ConnectionPage<TNode> | undefined)[]): Set<unknown> {
	return new Set(rowsOf(pages).map(idOf).filter((id) => id !== undefined))
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
 * Reports whether a query state answers the given variables rather than ones the query ran with before.
 * @param result - The query state.
 * @param variables - The variables the query runs with now.
 * @returns Whether the state belongs to them.
 */
function answers<TData>(result: UseQueryState<TData>, variables: object): boolean {
	return JSON.stringify(result.operation?.variables) === JSON.stringify(variables)
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
 * Returns the rows of the given pages in order, each node with an id listed once.
 * @param pages - The pages, the first possibly not loaded yet.
 * @returns The rows.
 */
function rowsOf<TNode>(pages: (ConnectionPage<TNode> | undefined)[]): TNode[] {
	const seen = new Set<unknown>()
	const rows: TNode[] = []
	for (const edge of pages.flatMap((page) => page?.edges ?? [])) {
		const id = idOf(edge.node)
		if (id === undefined || !seen.has(id)) {
			seen.add(id)
			rows.push(edge.node)
		}
	}
	return rows
}

/**
 * Returns the id a node carries.
 * @param node - The node.
 * @returns The id, undefined when the node has none.
 */
function idOf(node: unknown): unknown {
	return (node as { id?: unknown }).id
}

/**
 * Returns a loader holding no page past the first.
 * @param key - The variables the loader belongs to.
 * @returns The loader.
 */
function newLoader<TNode>(key: string): pageLoader<TNode> {
	return { ...noLaterPages<TNode>(key), generation: 0, running: false, wantMore: false, wantReread: false }
}

/**
 * Points the loader at new variables, dropping its pages, its waiting jobs and the run and read in flight.
 * @param loader - The loader.
 * @param key - The new variables.
 */
function reset<TNode>(loader: pageLoader<TNode>, key: string) {
	Object.assign(loader, noLaterPages<TNode>(key), { running: false, wantMore: false, wantReread: false })
	loader.generation += 1
}

/**
 * Returns the state the screen shows of a loader.
 * @param loader - The loader.
 * @returns The pages past the first beside the flags.
 */
function snapshot<TNode>(loader: pageLoader<TNode>): laterPages<TNode> {
	return { key: loader.key, pages: loader.pages, fetching: loader.fetching, failed: loader.failed }
}

/**
 * Returns the state of a connection holding no page past the first.
 * @param key - The variables the state belongs to.
 * @returns The empty state.
 */
function noLaterPages<TNode>(key: string): laterPages<TNode> {
	return { key, pages: [], fetching: false, failed: false }
}

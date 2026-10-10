import { useState } from 'react'
import { useQuery } from 'urql'
import type { AnyVariables, TypedDocumentNode } from 'urql'

/** ConnectionPage is one Relay page as the graph serializes it. */
export interface ConnectionPage<TNode> {
	edges: { node: TNode }[]
	pageInfo: { hasNextPage: boolean; endCursor?: string | null }
}

/** ConnectionResult carries the rows beside the load more controls. */
export interface ConnectionResult<TNode, TData = unknown> {
	/** rows are every node of the window, in the order the graph answered. */
	rows: TNode[]
	/** data is the whole document the connection was selected from. */
	data: TData | undefined
	/** isPending reports that the first window has not arrived. */
	isPending: boolean
	/** isError reports that the window could not be loaded. */
	isError: boolean
	/** hasNextPage reports that a bigger window is available. */
	hasNextPage: boolean
	/** isFetchingNextPage reports that a bigger window is in flight. */
	isFetchingNextPage: boolean
	/** fetchNextPage grows the window by one page. */
	fetchNextPage: () => Promise<void>
}

/** grownState carries how many pages the window holds beside the variables it belongs to. */
interface grownState {
	key: string
	pages: number
}

/**
 * Reads one Relay connection as a window growing a page at a time, so a refetch reads every row again.
 * @param options - The document, its variables with the page size as first, the page selector, the pause flag,
 * and the most rows one read may ask for.
 * @returns The rows beside the load more controls.
 */
export function useConnection<TData, TVariables extends AnyVariables & { first: number }, TNode>(options: {
	query: TypedDocumentNode<TData, TVariables>
	variables: TVariables
	select: (data: TData) => ConnectionPage<TNode> | null | undefined
	pause?: boolean
	ceiling?: number
}): ConnectionResult<TNode, TData> {
	const key = JSON.stringify(options.variables)
	const [grown, setGrown] = useState<grownState>({ key, pages: 1 })
	const pages = grown.key === key ? grown.pages : 1
	const first = windowSize(options.variables.first, pages, options.ceiling)
	const [result] = useQuery<TData, TVariables>({
		query: options.query,
		variables: { ...options.variables, first },
		pause: options.pause,
		requestPolicy: 'cache-and-network',
	})
	const loaded = result.data ? options.select(result.data) : undefined
	const listed = loaded ?? emptyPage<TNode>()
	return {
		rows: listed.edges.map((edge) => edge.node),
		data: result.data,
		isPending: result.fetching && loaded === undefined,
		isError: result.error !== undefined,
		hasNextPage: offersMore(listed, first, options.ceiling),
		isFetchingNextPage: result.fetching && pages > 1,
		fetchNextPage: async () => {
			if (loaded) {
				setGrown({ key, pages: pages + 1 })
			}
		},
	}
}

/**
 * Returns how many rows a window of the given pages asks for, never past the ceiling.
 * @param pageSize - The rows one page holds.
 * @param pages - The pages the window holds.
 * @param ceiling - The most rows one read may ask for, if known.
 * @returns The window size.
 */
function windowSize(pageSize: number, pages: number, ceiling: number | undefined): number {
	return Math.min(pageSize * pages, ceiling ?? Number.POSITIVE_INFINITY)
}

/**
 * Reports whether a bigger window is worth asking for.
 * @param page - The page last answered.
 * @param first - The rows the window asked for.
 * @param ceiling - The most rows one read may ask for, if known.
 * @returns Whether to offer more.
 */
function offersMore<TNode>(page: ConnectionPage<TNode>, first: number, ceiling: number | undefined): boolean {
	return page.pageInfo.hasNextPage && (ceiling === undefined || first < ceiling)
}

/**
 * Returns the page standing in for a connection that has not loaded.
 * @returns The empty page.
 */
function emptyPage<TNode>(): ConnectionPage<TNode> {
	return { edges: [], pageInfo: { hasNextPage: false, endCursor: undefined } }
}

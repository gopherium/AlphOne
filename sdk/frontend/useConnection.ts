// SPDX-License-Identifier: AGPL-3.0-or-later

import { useState } from 'react'
import { useQuery } from 'urql'
import type { AnyVariables, TypedDocumentNode } from 'urql'

/** ConnectionPage is one Relay page as the graph serializes it. */
export interface ConnectionPage<TNode> {
	edges: { node: TNode }[]
	pageInfo: { hasNextPage: boolean; endCursor?: string | null }
}

/** ConnectionResult carries the merged rows beside the load more controls. */
export interface ConnectionResult<TNode> {
	/** rows are every node loaded so far, oldest page first. */
	rows: TNode[]
	/** isPending reports that the first page has not arrived. */
	isPending: boolean
	/** isError reports that the page could not be loaded. */
	isError: boolean
	/** hasNextPage reports that another page is available. */
	hasNextPage: boolean
	/** isFetchingNextPage reports that a further page is in flight. */
	isFetchingNextPage: boolean
	/** fetchNextPage advances to the next page. */
	fetchNextPage: () => Promise<void>
}

/** cursorState carries the cursor beside the variables it belongs to. */
interface cursorState {
	key: string
	after: string | undefined
}

/**
 * Pages one Relay connection, merging every page through the normalized cache.
 * @param options - The document, its variables, the page selector, and the pause flag.
 * @returns The merged rows beside the load more controls.
 */
export function useConnection<TData, TVariables extends AnyVariables, TNode>(options: {
	query: TypedDocumentNode<TData, TVariables>
	variables: TVariables & { after?: string | null }
	select: (data: TData) => ConnectionPage<TNode> | null | undefined
	pause?: boolean
}): ConnectionResult<TNode> {
	const key = JSON.stringify(options.variables)
	const [cursor, setCursor] = useState<cursorState>({ key, after: undefined })
	const after = cursor.key === key ? cursor.after : undefined
	const [result] = useQuery<TData, TVariables>({
		query: options.query,
		variables: { ...options.variables, after },
		pause: options.pause,
		requestPolicy: 'cache-and-network',
	})
	const loaded = result.data ? options.select(result.data) : undefined
	const page = loaded ?? emptyPage<TNode>()
	return {
		rows: page.edges.map((edge) => edge.node),
		isPending: result.fetching && loaded === undefined,
		isError: result.error !== undefined,
		hasNextPage: page.pageInfo.hasNextPage,
		isFetchingNextPage: result.fetching && after !== undefined,
		fetchNextPage: async () => {
			setCursor({ key, after: page.pageInfo.endCursor ?? undefined })
		},
	}
}

/**
 * Returns the page standing in for a connection that has not loaded.
 * @returns The empty page.
 */
function emptyPage<TNode>(): ConnectionPage<TNode> {
	return { edges: [], pageInfo: { hasNextPage: false, endCursor: undefined } }
}

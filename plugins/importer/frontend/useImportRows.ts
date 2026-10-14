// SPDX-License-Identifier: AGPL-3.0-or-later

import { useAdminSettings, useGraph } from '@alphone/frontend-sdk'
import type { GraphClient } from '@alphone/frontend-sdk'
import { useEffect, useRef, useState } from 'react'

import type { ImportRowsQuery } from './gql/graphql'
import { importRowsQuery } from './operations'

/** ImportRow is one staged row as the rows document selects it. */
export type ImportRow = NonNullable<ImportRowsQuery['importJob']>['rows'][number]

/** ImportRowsRead carries every staged row of an import beside whether they could be read. */
export interface ImportRowsRead {
	/** rows are every staged row in file order, undefined until the first read finishes. */
	rows: ImportRow[] | undefined
	/** failed reports that a page of rows could not be read. */
	failed: boolean
}

/** rowsRead is one finished read of an import's rows, tied to the import it belongs to. */
interface rowsRead extends ImportRowsRead {
	importId: string
}

/** refetchedBy names the operations whose refetch reads the rows again. */
const refetchedBy = ['ImportRows', 'ImportDetail']

/**
 * Reads every staged row of an import a page at a time, each the largest list page, again on every refetch.
 * @param importId - The import whose rows are read.
 * @returns The rows beside whether they could be read.
 */
export function useImportRows(importId: string): ImportRowsRead {
	const graph = useGraph()
	const { settings, failed: unsized } = useAdminSettings()
	const chunk = settings === undefined ? undefined : Math.max(...settings.listPageSizes)
	const ready = settings !== undefined || unsized
	const generation = useRefetchCount(graph)
	const [read, setRead] = useState<rowsRead>({ importId, rows: undefined, failed: false })
	const previous = useRef<Promise<unknown>>(Promise.resolve())
	useEffect(() => {
		if (!ready) {
			return undefined
		}
		let live = true
		const pages: pageReader = { client: graph.client, importId, chunk, isLive: () => live }
		const started = previous.current.then(() => readFrom(pages, []))
		previous.current = started.catch(() => undefined)
		started.then(
			(rows) => {
				if (live) {
					setRead({ importId, rows, failed: false })
				}
			},
			() => {
				if (live) {
					setRead({ importId, rows: undefined, failed: true })
				}
			},
		)
		return () => {
			live = false
		}
	}, [graph, importId, chunk, ready, generation])
	return read.importId === importId ? { rows: read.rows, failed: read.failed } : { rows: undefined, failed: false }
}

/** pageReader is what one read of an import's rows pages through, and whether the screen still wants it. */
interface pageReader {
	client: GraphClient['client']
	importId: string
	chunk: number | undefined
	isLive: () => boolean
}

/**
 * Counts the refetches that name the rows, so a read can start again on each.
 * @param graph - The graph whose refetches are heard.
 * @returns How many such refetches there have been.
 */
function useRefetchCount(graph: GraphClient): number {
	const [count, setCount] = useState(0)
	useEffect(
		() =>
			graph.onRefetch((operations) => {
				if (refetchedBy.some((name) => operations.includes(name))) {
					setCount((current) => current + 1)
				}
			}),
		[graph],
	)
	return count
}

/**
 * Reads the pages of rows past the ones already read until a page comes back short or empty, or the screen moves on.
 * @param pages - The import, the page size and the client the pages are read through.
 * @param read - The rows read so far.
 * @returns Every row of the import in file order, or the ones read when the screen moved on.
 */
async function readFrom(pages: pageReader, read: ImportRow[]): Promise<ImportRow[]> {
	const result = await pages.client
		.query(
			importRowsQuery,
			{ id: pages.importId, limit: pages.chunk, offset: read.length },
			{ requestPolicy: 'network-only' },
		)
		.toPromise()
	if (result.error !== undefined) {
		throw result.error
	}
	const page = result.data?.importJob?.rows ?? []
	const rows = [...read, ...page]
	return isLastPage(page.length, pages.chunk) || !pages.isLive() ? rows : readFrom(pages, rows)
}

/**
 * Reports whether a page of rows is the last one, being empty or shorter than it asked for.
 * @param length - How many rows the page holds.
 * @param chunk - The rows the page asked for, undefined when the server chose.
 * @returns True for the last page.
 */
function isLastPage(length: number, chunk: number | undefined): boolean {
	return length === 0 || (chunk !== undefined && length < chunk)
}

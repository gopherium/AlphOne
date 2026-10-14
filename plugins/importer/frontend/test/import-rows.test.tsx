// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, paging, server } from '@alphone/frontend-sdk/testing'
import { act, renderHook, waitFor } from '@testing-library/react'
import { expect, test } from 'vitest'

import { importID, rowsPage } from '../handlers'
import { useImportRows } from '../useImportRows'
import type { ImportRowsRead } from '../useImportRows'
import { hosting } from './harness'

/** otherImportID names a second import a screen may move to. */
const otherImportID = '019f5a00-0000-7000-8000-000000000002'

/** pageAsked is the page one rows request asked for. */
type pageAsked = { id: string; limit?: number | null; offset: number }

/**
 * Builds the staged rows of an import holding count rows, each still pending.
 * @param count - How many rows the import holds.
 * @param outcome - The outcome every row carries.
 * @returns The rows in file order.
 */
function stagedRows(count: number, outcome = 'pending') {
	return Array.from({ length: count }, (_, index) => ({
		__typename: 'ImportRow',
		id: `019f5a00-0000-7000-8000-0000000002${String(index).padStart(2, '0')}`,
		position: index + 1,
		cells: [`Row ${index + 1}`],
		outcome,
		reason: null,
	}))
}

/**
 * Serves the rows of an import page by page, recording every page asked for.
 * @param rows - Every staged row of the import.
 * @returns The pages asked for, in order.
 */
function servingRows(rows: readonly Record<string, unknown>[]): pageAsked[] {
	const asked: pageAsked[] = []
	server.use(
		graphql.query('ImportRows', ({ variables }) => {
			asked.push(variables as pageAsked)
			return HttpResponse.json(rowsPage(rows, variables as pageAsked))
		}),
	)
	return asked
}

/**
 * Hosts the rows hook for an import, answering the hook's result and the graph it reads through.
 * @param importId - The import whose rows are read.
 * @returns The rendered hook and its graph.
 */
function hostRows(importId = importID) {
	const { Hosted, graph } = hosting()
	const seen: ImportRowsRead[] = []
	const hook = renderHook(
		(props: { importId: string }) => {
			const read = useImportRows(props.importId)
			seen.push(read)
			return read
		},
		{ wrapper: Hosted, initialProps: { importId } },
	)
	return { ...hook, graph, seen }
}

/**
 * Returns the positions of the rows a read answered.
 * @param rows - The rows, undefined before the first read finishes.
 * @returns The positions in order.
 */
function positionsOf(rows: readonly { position: number }[] | undefined) {
	return rows?.map((row) => row.position)
}

/**
 * Returns the outcomes of the rows a read answered.
 * @param rows - The rows, undefined before the first read finishes.
 * @returns The outcomes in order.
 */
function outcomesOf(rows: readonly { outcome: string }[] | undefined) {
	return rows?.map((row) => row.outcome)
}

test('reads every row a page at a time, each the largest list page', async () => {
	paging([1, 2], 1)
	const asked = servingRows(stagedRows(5))

	const { result } = hostRows()

	await waitFor(() => expect(positionsOf(result.current.rows)).toEqual([1, 2, 3, 4, 5]))
	expect(asked.map((page) => [page.limit, page.offset])).toEqual([
		[2, 0],
		[2, 2],
		[2, 4],
	])
	expect(result.current.failed).toBe(false)
})

test('reads one empty page more when the rows fill the last page exactly', async () => {
	paging([2], 2)
	const asked = servingRows(stagedRows(4))

	const { result } = hostRows()

	await waitFor(() => expect(positionsOf(result.current.rows)).toEqual([1, 2, 3, 4]))
	expect(asked.map((page) => page.offset)).toEqual([0, 2, 4])
})

test('lets the server choose the page when the list settings cannot be read, stopping on an empty page', async () => {
	server.use(
		graphql.query('AdminSettings', () => HttpResponse.json({ data: null, errors: [{ message: 'down' }] })),
	)
	const asked = servingRows(stagedRows(3))

	const { result } = hostRows()

	await waitFor(() => expect(positionsOf(result.current.rows)).toEqual([1, 2, 3]))
	expect(asked.map((page) => [page.limit ?? 'unset', page.offset])).toEqual([
		['unset', 0],
		['unset', 3],
	])
})

test('reports the rows unread when a page fails', async () => {
	paging([2], 2)
	server.use(
		graphql.query('ImportRows', ({ variables }) =>
			(variables as pageAsked).offset === 0
				? HttpResponse.json(rowsPage(stagedRows(5), variables as pageAsked))
				: HttpResponse.json({ data: null, errors: [{ message: 'gone' }] }),
		),
	)

	const { result } = hostRows()

	await waitFor(() => expect(result.current.failed).toBe(true))
	expect(result.current.rows).toBeUndefined()
})

test('reads no rows for an import that is gone', async () => {
	server.use(graphql.query('ImportRows', () => HttpResponse.json({ data: { importJob: null } })))

	const { result } = hostRows()

	await waitFor(() => expect(result.current.rows).toEqual([]))
})

test('reads every row again when the import is refetched, and not for another list', async () => {
	paging([2], 2)
	let outcome = 'pending'
	let reads = 0
	server.use(
		graphql.query('ImportRows', ({ variables }) => {
			reads += 1
			return HttpResponse.json(rowsPage(stagedRows(3, outcome), variables as pageAsked))
		}),
	)
	const { result, graph } = hostRows()
	await waitFor(() => expect(result.current.rows?.[0]?.outcome).toBe('pending'))
	const firstReads = reads

	act(() => graph.refetch(['Imports']))
	outcome = 'imported'
	act(() => graph.refetch(['ImportDetail', 'Imports']))

	await waitFor(() => expect(outcomesOf(result.current.rows)).toEqual(['imported', 'imported', 'imported']))
	expect(reads).toBe(firstReads * 2)
})

test('answers no rows for an import it moved to until that import is read', async () => {
	let release: (() => void) | undefined
	server.use(
		graphql.query('ImportRows', async ({ variables }) => {
			if ((variables as pageAsked).id === otherImportID) {
				await new Promise<void>((resolve) => {
					release = resolve
				})
			}
			return HttpResponse.json(rowsPage(stagedRows(1), variables as pageAsked))
		}),
	)
	const { result, rerender } = hostRows()
	await waitFor(() => expect(result.current.rows).toHaveLength(1))

	rerender({ importId: otherImportID })

	expect(result.current.rows).toBeUndefined()
	await waitFor(() => expect(release).toBeDefined())
	release?.()
	await waitFor(() => expect(result.current.rows).toHaveLength(1))
})

test('a read the refetch replaced stops, and the next read starts once it has', async () => {
	for (const stale of [
		(variables: pageAsked) => rowsPage(stagedRows(3, 'pending'), variables),
		() => ({ data: null, errors: [{ message: 'gone' }] }),
	]) {
		paging([2], 2)
		let release: (() => void) | undefined
		const asked: number[] = []
		server.use(
			graphql.query('ImportRows', async ({ variables }) => {
				asked.push((variables as pageAsked).offset)
				if (asked.length === 1) {
					await new Promise<void>((resolve) => {
						release = resolve
					})
					return HttpResponse.json(stale(variables as pageAsked))
				}
				await new Promise((resolve) => setTimeout(resolve, 30))
				return HttpResponse.json(rowsPage(stagedRows(3, 'imported'), variables as pageAsked))
			}),
		)
		const { result, graph, seen } = hostRows()
		await waitFor(() => expect(release).toBeDefined())

		act(() => graph.refetch(['ImportDetail']))
		await new Promise((resolve) => setTimeout(resolve, 20))
		expect(asked).toEqual([0])
		release?.()

		await waitFor(() => expect(outcomesOf(result.current.rows)).toEqual(['imported', 'imported', 'imported']))
		expect(asked).toEqual([0, 0, 2])
		expect(seen.some((read) => read.failed || read.rows?.some((row) => row.outcome === 'pending'))).toBe(false)
	}
})

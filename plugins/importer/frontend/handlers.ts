// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql } from 'msw'

/**
 * Returns the raw body of a multipart graph request.
 * @param request - The posted request.
 * @returns The body, or an empty string for a request that is not multipart.
 */
export async function multipartBody(request: Request): Promise<string> {
	if (!request.headers.get('content-type')?.startsWith('multipart/form-data')) {
		return ''
	}
	return request.clone().text()
}

export const importID = '019f5a00-0000-7000-8000-000000000001'

/** defaultRowsPage is the rows a fixture page answers when the request names no limit. */
const defaultRowsPage = 50

/**
 * Returns one page of the staged rows the way the server pages them, its own page for a request naming no limit.
 * @param rows - Every staged row of the import.
 * @param variables - The limit and the offset the request asks for.
 * @returns The rows document body for that page.
 */
export function rowsPage(
	rows: readonly Record<string, unknown>[],
	variables: { limit?: number | null; offset: number },
) {
	const end = variables.offset + (variables.limit ?? defaultRowsPage)
	return { data: { importJob: { __typename: 'ImportJob', id: importID, rows: rows.slice(variables.offset, end) } } }
}

const importFields = [
	{ __typename: 'ImportField', name: 'name', label: 'Name', required: true },
	{ __typename: 'ImportField', name: 'email', label: 'Email', required: false },
	{ __typename: 'ImportField', name: 'phone', label: 'Phone', required: false },
]

const stagedRows = [
	{
		__typename: 'ImportRow',
		id: '019f5a00-0000-7000-8000-000000000101',
		position: 1,
		cells: ['Maria Perez', 'maria@example.com'],
		outcome: 'pending',
		reason: null,
	},
	{
		__typename: 'ImportRow',
		id: '019f5a00-0000-7000-8000-000000000102',
		position: 2,
		cells: ['Ana Lopez', 'ana@example.com'],
		outcome: 'pending',
		reason: null,
	},
]

export const handlers = [
	graphql.query('Imports', () =>
		HttpResponse.json({
			data: {
				imports: [
					{
						__typename: 'ImportJob',
						id: importID,
						filename: 'contacts.csv',
						state: 'ready',
						rowCount: 2,
						importedCount: 0,
						skippedCount: 0,
						failedCount: 0,
						createdAt: '2026-08-01T10:00:00Z',
					},
				],
			},
		}),
	),
	graphql.query('ImportDetail', () =>
		HttpResponse.json({
			data: {
				importJob: {
					__typename: 'ImportJob',
					id: importID,
					filename: 'contacts.csv',
					state: 'ready',
					columns: ['Name', 'Email'],
					mapping: [],
				},
				importFields,
			},
		}),
	),
	graphql.query('ImportRows', ({ variables }) =>
		HttpResponse.json(rowsPage(stagedRows, variables as { limit?: number | null; offset: number })),
	),
]

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
					rows: stagedRows,
				},
				importFields,
			},
		}),
	),
]

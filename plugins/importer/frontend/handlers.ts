// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, http } from 'msw'

export const importID = '019f5a00-0000-7000-8000-000000000001'

const storedImport = {
	id: importID,
	user_id: '019f5a00-0000-7000-8000-0000000000aa',
	filename: 'contacts.csv',
	state: 'ready',
	row_count: 2,
	imported_count: 0,
	skipped_count: 0,
	failed_count: 0,
	created_at: '2026-08-01T10:00:00Z',
}

export const handlers = [
	http.get('/api/plugins/importer/fields', () =>
		HttpResponse.json([
			{ name: 'name', label: 'Name', required: true },
			{ name: 'email', label: 'Email', required: false },
			{ name: 'phone', label: 'Phone', required: false },
		]),
	),
	http.get('/api/plugins/importer/imports', () => HttpResponse.json([storedImport])),
	http.get('/api/plugins/importer/imports/:id', () =>
		HttpResponse.json({
			...storedImport,
			columns: ['Name', 'Email'],
			mapping: {},
		}),
	),
	http.get('/api/plugins/importer/imports/:id/rows', () =>
		HttpResponse.json([
			{
				id: '019f5a00-0000-7000-8000-000000000101',
				position: 1,
				cells: ['Maria Perez', 'maria@example.com'],
				outcome: 'pending',
				reason: null,
				contact_id: null,
			},
			{
				id: '019f5a00-0000-7000-8000-000000000102',
				position: 2,
				cells: ['Ana Lopez', 'ana@example.com'],
				outcome: 'pending',
				reason: null,
				contact_id: null,
			},
		]),
	),
]

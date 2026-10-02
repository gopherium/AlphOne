// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { DataViews, filterSortAndPaginate, type Field, type View } from '@alphone/frontend-sdk/dataviews'

type contactRow = { id: string; name: string }

test('filterSortAndPaginate searches, sorts and pages the rows a list holds in the browser', () => {
	const fields: Field<contactRow>[] = [{ id: 'name', label: 'Name', type: 'text', enableGlobalSearch: true }]
	const rows = [
		{ id: '1', name: 'Maria Perez' },
		{ id: '2', name: 'Ada Lovelace' },
		{ id: '3', name: 'Maria Lopez' },
	]
	const view: View = {
		type: 'table',
		search: 'maria',
		sort: { field: 'name', direction: 'asc' },
		page: 2,
		perPage: 1,
	}

	const shown = filterSortAndPaginate(rows, view, fields)

	expect(shown.data).toEqual([{ id: '1', name: 'Maria Perez' }])
	expect(shown.paginationInfo).toEqual({ totalItems: 2, totalPages: 2 })
})

test('DataViews renders the rows it is handed', () => {
	const fields: Field<contactRow>[] = [{ id: 'name', label: 'Name' }]
	const view: View = { type: 'table', fields: ['name'], page: 1, perPage: 10 }

	render(
		<DataViews<contactRow>
			data={[{ id: '1', name: 'Maria Perez' }]}
			fields={fields}
			view={view}
			onChangeView={() => {}}
			paginationInfo={{ totalItems: 1, totalPages: 1 }}
			defaultLayouts={{ table: {} }}
			getItemId={(item) => item.id}
		/>,
	)

	expect(screen.getByText('Maria Perez')).toBeInTheDocument()
})

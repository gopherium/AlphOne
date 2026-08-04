// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { DataViews, type Field, type View } from '@alphone/frontend-sdk/dataviews'

type contactRow = { id: string; name: string }

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

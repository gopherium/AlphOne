// SPDX-License-Identifier: AGPL-3.0-or-later

import { DataViews, type Field, type View } from '@alphone/frontend-sdk/dataviews'
import { useState } from 'react'

import type { ImportDetail, ImportRow } from './api'

type previewRow = {
	id: string
	position: number
	outcome: string
	reason: string
	cells: string[]
}

/**
 * Builds the preview rows a staged import shows.
 * @param rows - The staged rows.
 * @returns The rows in preview shape.
 */
function previewRows(rows: ImportRow[]): previewRow[] {
	return rows.map((row) => ({
		id: row.id,
		position: row.position,
		outcome: row.outcome,
		reason: row.reason ?? '',
		cells: row.cells,
	}))
}

/**
 * Builds one field per column of the import beside its outcome fields.
 * @param columns - The column list of the import.
 * @returns The fields the table renders.
 */
function previewFields(columns: string[]): Field<previewRow>[] {
	const cells: Field<previewRow>[] = columns.map((column, index) => ({
		id: `cell-${index}`,
		label: column === '' ? `Column ${index + 1}` : column,
		getValue: ({ item }: { item: previewRow }) => item.cells[index] ?? '',
	}))
	return [
		{ id: 'position', label: 'Row', getValue: ({ item }) => String(item.position) },
		...cells,
		{ id: 'outcome', label: 'Outcome', getValue: ({ item }) => item.outcome },
		{ id: 'reason', label: 'Reason', getValue: ({ item }) => item.reason },
	]
}

/**
 * Renders the staged rows of an import as a filterable table.
 * @returns The rows table.
 */
export default function RowsTable({
	stored,
	rows,
}: {
	stored: ImportDetail
	rows: ImportRow[]
}) {
	const fields = previewFields(stored.columns)
	const [view, setView] = useState<View>({
		type: 'table',
		fields: fields.map((field) => field.id),
		page: 1,
		perPage: 25,
	})
	const data = previewRows(rows)

	return (
		<div
			className="godmin-table-scroll"
			role="region"
			aria-label="Rows"
			tabIndex={0}
			style={{ minWidth: 0, maxWidth: '100%' }}
		>
			<DataViews<previewRow>
				data={data}
				fields={fields}
				view={view}
				onChangeView={setView}
				paginationInfo={{ totalItems: data.length, totalPages: 1 }}
				defaultLayouts={{ table: {} }}
				getItemId={(item) => item.id}
			/>
		</div>
	)
}

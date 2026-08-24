// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, _x, sprintf } from '@alphone/frontend-sdk'
import { DataViews, type Field, type View } from '@alphone/frontend-sdk/dataviews'
import { useState } from 'react'

import type { StoredImport } from './ImportScreen'

/** ImportRow is one staged row as the detail document selects it. */
type ImportRow = StoredImport['rows'][number]

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
function previewRows(rows: readonly ImportRow[]): previewRow[] {
	return rows.map((row) => ({
		id: row.id,
		position: row.position,
		outcome: row.outcome,
		reason: row.reason ?? '',
		cells: row.cells,
	}))
}

/**
 * Returns the label each row outcome carries, read fresh so the loaded catalogue answers.
 * @returns The labels, keyed by the outcome the server names.
 */
function outcomeLabels(): Record<string, string> {
	return {
		pending: _x('Pending', 'row outcome', 'alphone-importer'),
		imported: _x('Imported', 'row outcome', 'alphone-importer'),
		skipped: _x('Skipped', 'row outcome', 'alphone-importer'),
		failed: _x('Failed', 'row outcome', 'alphone-importer'),
	}
}

/**
 * Builds one field per column of the import beside its outcome fields.
 * @param columns - The column list of the import.
 * @returns The fields the table renders.
 */
function previewFields(columns: readonly string[]): Field<previewRow>[] {
	const cells: Field<previewRow>[] = columns.map((column, index) => ({
		id: `cell-${index}`,
		label: column === '' ? sprintf(__('Column %(number)d', 'alphone-importer'), { number: index + 1 }) : column,
		getValue: ({ item }: { item: previewRow }) => item.cells[index] ?? '',
	}))
	return [
		{ id: 'position', label: __('Row', 'alphone-importer'), getValue: ({ item }) => String(item.position) },
		...cells,
		{
			id: 'outcome',
			label: __('Outcome', 'alphone-importer'),
			getValue: ({ item }) => outcomeLabels()[item.outcome] ?? item.outcome,
		},
		{ id: 'reason', label: __('Reason', 'alphone-importer'), getValue: ({ item }) => item.reason },
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
	stored: StoredImport
	rows: readonly ImportRow[]
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
			aria-label={__('Rows', 'alphone-importer')}
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

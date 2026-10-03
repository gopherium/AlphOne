// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, _x, formatNumber, useAdminSettings } from '@alphone/frontend-sdk'
import { DataViews, filterSortAndPaginate, type Field, type View } from '@alphone/frontend-sdk/dataviews'
import { useMemo, useState } from 'react'

import { columnLabel } from './column'
import { fieldLabeller } from './fieldLabels'
import type { ImportField, StoredImport } from './ImportScreen'
import { rowReasonText } from './rowReasons'

/** ImportRow is one staged row as the detail document selects it. */
type ImportRow = StoredImport['rows'][number]

/** PreviewRow is one staged row as the preview table shows it. */
type PreviewRow = {
	id: string
	line: number
	outcome: string
	reason: string
	cells: string[]
}

/** outcomes lists every outcome a staged row reaches, in the order the filter offers them. */
const outcomes = ['pending', 'imported', 'skipped', 'failed']

/** opening is the view the preview opens on, the rows in the order the file holds them. */
const opening: View = { type: 'table', page: 1, sort: { field: 'line', direction: 'asc' } }

/**
 * Builds the preview rows a staged import shows.
 * @param rows - The staged rows.
 * @param labelOf - The labeller naming a field the way the reader sees it.
 * @returns The rows in preview shape, each reason in the reader's language.
 */
function previewRows(rows: readonly ImportRow[], labelOf: (name: string) => string): PreviewRow[] {
	return rows.map((row) => ({
		id: row.id,
		line: row.position,
		outcome: row.outcome,
		reason: rowReasonText(row.reason, labelOf),
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
 * Builds one text field per column of the import beside its row number and outcome fields.
 * @param columns - The column list of the import.
 * @returns The fields the table renders.
 */
function previewFields(columns: readonly string[]): Field<PreviewRow>[] {
	const labels = outcomeLabels()
	const cells: Field<PreviewRow>[] = columns.map((column, index) => ({
		id: `cell-${index}`,
		label: column === '' ? columnLabel(index) : column,
		type: 'text',
		getValue: ({ item }: { item: PreviewRow }) => item.cells[index] ?? '',
		enableGlobalSearch: true,
		filterBy: false,
	}))
	return [
		{
			id: 'line',
			label: __('Row', 'alphone-importer'),
			type: 'integer',
			render: ({ item }) => formatNumber(item.line),
			filterBy: false,
		},
		...cells,
		{
			id: 'outcome',
			label: __('Outcome', 'alphone-importer'),
			type: 'text',
			elements: outcomes.map((outcome) => ({ value: outcome, label: labels[outcome] })),
			filterBy: { operators: ['isAny'] },
			render: ({ item }) => labels[item.outcome] ?? item.outcome,
		},
		{
			id: 'reason',
			label: __('Reason', 'alphone-importer'),
			type: 'text',
			render: ({ item }) => <span className="alphone-import__reason">{item.reason}</span>,
			enableGlobalSearch: true,
			filterBy: false,
		},
	]
}

/**
 * Renders the staged rows of an import as a table to search, filter, sort and page.
 * @returns The rows table.
 */
export default function RowsTable({
	stored,
	rows,
	fields: importFields,
}: {
	stored: StoredImport
	rows: readonly ImportRow[]
	fields: readonly ImportField[]
}) {
	const fields = useMemo(() => previewFields(stored.columns), [stored.columns])
	const labelOf = useMemo(() => fieldLabeller(importFields), [importFields])
	const { settings, failed } = useAdminSettings()
	const [changed, setChanged] = useState<View>(opening)
	const view: View = {
		fields: fields.map((field) => field.id),
		...changed,
		perPage: changed.perPage ?? settings?.listPageSize,
	}
	const data = useMemo(() => previewRows(rows, labelOf), [rows, labelOf])
	const shown = filterSortAndPaginate(data, view, fields)
	const sizing = view.perPage === undefined && !failed

	return (
		<section aria-label={__('Rows', 'alphone-importer')} className="godmin-list">
			<DataViews<PreviewRow>
				data={shown.data}
				fields={fields}
				view={view}
				onChangeView={setChanged}
				paginationInfo={shown.paginationInfo}
				defaultLayouts={{ table: {} }}
				getItemId={(item) => item.id}
				isLoading={sizing}
				searchLabel={__('Search rows…', 'alphone-importer')}
				config={settings === undefined ? undefined : { perPageSizes: settings.listPageSizes }}
			/>
		</section>
	)
}

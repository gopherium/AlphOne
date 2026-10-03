// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberFormatLocale } from '@alphone/frontend-sdk'
import { HttpResponse, graphql, paging, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import type { ImportField, StoredImport } from '../ImportScreen'
import RowsTable from '../RowsTable'
import { inSpanish, renderHosted } from './harness'

/** StagedRow is one staged row as the detail document selects it. */
type StagedRow = StoredImport['rows'][number]

/**
 * Builds one staged row.
 * @param position - Its place in the file, counted from one.
 * @param cells - The cells it holds.
 * @param changes - The fields that differ from an imported row.
 * @returns The row.
 */
function rowOf(position: number, cells: string[], changes: Partial<StagedRow> = {}): StagedRow {
	return {
		id: `019f5a00-0000-7000-8000-000000000${100 + position}`,
		position,
		cells,
		outcome: 'imported',
		reason: null,
		...changes,
	}
}

const maria = rowOf(1, ['Maria Perez', 'maria@example.com'])
const ana = rowOf(2, ['Ana Lopez'], {
	outcome: 'skipped',
	reason: { code: 'identity_taken_by', meta: { ownerName: 'Ana Lopez' } },
})
const lucia = rowOf(3, ['Lucia Gomez', 'lucia@example.com'], {
	outcome: 'failed',
	reason: { code: 'row_quote_misplaced', meta: { line: 4 } },
})

const rows: StoredImport['rows'] = [maria, ana]

const stored: StoredImport = {
	id: '019f5a00-0000-7000-8000-000000000001',
	filename: 'contacts.csv',
	state: 'ready',
	columns: ['Name', ''],
	mapping: [],
	rows,
}

/**
 * Renders the preview of the given rows for a signed-in admin.
 * @param shown - The staged rows to preview.
 * @param fields - The live fields the import offers.
 */
function renderRows(shown: readonly StagedRow[] = rows, fields: readonly ImportField[] = []) {
	renderHosted(<RowsTable stored={stored} rows={shown} fields={fields} />)
}

/**
 * Returns the first cell of every row the preview shows, in order.
 * @returns The names.
 */
async function shownNames() {
	await screen.findByRole('table')
	return screen
		.getAllByRole('row')
		.slice(1)
		.map((row) => within(row).getAllByRole('cell')[1].textContent)
}

test('the preview shows every cell beside its outcome and reason', async () => {
	renderRows()

	expect(await screen.findByText('Maria Perez')).toBeInTheDocument()
	expect(screen.getByText('maria@example.com')).toBeInTheDocument()
	expect(screen.getByText('Imported')).toBeInTheDocument()
	expect(screen.getByText('Ana Lopez already holds an address in this row.')).toBeInTheDocument()
})

test('a reason names a field by its label and its kind by the label the kind reads as', async () => {
	const failed = rowOf(4, ['Maria Perez', 'maria@example.com'], {
		outcome: 'failed',
		reason: { code: 'value_kind_mismatch', meta: { field: 'birthDate', kind: 'DATE' } },
	})
	renderRows([failed], [{ name: 'birthDate', label: 'Birth date', required: false }])

	expect(
		await screen.findByText('The value for Birth date does not match the kind the field declares: Date.'),
	).toBeInTheDocument()
	expect(screen.queryByText(/birthDate|DATE/)).not.toBeInTheDocument()
})

test('a reason reads in Spanish and the search finds it by its Spanish words', async () => {
	await inSpanish()
	renderRows([maria, ana, lucia])

	expect(await screen.findByText('Ana Lopez ya tiene una dirección de esta fila.')).toBeInTheDocument()
	await userEvent.type(screen.getByRole('searchbox', { name: 'Buscar filas…' }), 'comillas')

	await waitFor(async () => expect(await shownNames()).toEqual(['Lucia Gomez']))
})

test('the counts a reason names are written in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	const short = rowOf(4, ['Maria Perez'], {
		reason: { code: 'row_cell_count_mismatch', meta: { cells: 1, columns: 2 } },
	})
	renderRows([short])

	expect(
		await screen.findByText('The row does not match the header. Cells in the row: १. Columns in the header: २.'),
	).toBeInTheDocument()
})

test('a reason the interface does not know reads as the server named it', async () => {
	renderRows([{ ...maria, outcome: 'failed', reason: { code: 'row_from_the_future', meta: {} } }])

	expect(await screen.findByText('row_from_the_future')).toBeInTheDocument()
})

test('the preview leaves the sideways scroll to the list, with no tab stop of its own around it', async () => {
	renderRows()

	const region = await screen.findByRole('region', { name: 'Rows' })
	const search = await within(region).findByRole('searchbox', { name: 'Search rows…' })
	expect(region).not.toHaveAttribute('tabindex')
	expect(region.closest('.godmin-table-scroll')).toBeNull()
	expect(search.closest('.dataviews-wrapper')).not.toBeNull()
})

test('the preview lines its search and cells up with the page text around it', async () => {
	renderRows()

	const region = await screen.findByRole('region', { name: 'Rows' })
	expect(region).toHaveClass('godmin-list')
})

test('the preview adds no first level heading to the screen it sits in', async () => {
	renderRows()

	await screen.findByText('Maria Perez')
	expect(
		screen.queryAllByRole('heading').filter((heading) => heading.tagName === 'H1'),
	).toHaveLength(0)
})

test('a blank header is named after its position in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	renderRows()

	expect(await screen.findByText('Column २')).toBeInTheDocument()
})

test('a row number past a thousand is written in the format locale, set on the right', async () => {
	renderRows([{ ...maria, position: 1234 }])

	expect(await screen.findByText('1.234')).toBeInTheDocument()
	expect(screen.getByRole('columnheader', { name: /Row/ })).toHaveStyle({ textAlign: 'end' })
})

test('a row shorter than the header leaves its missing cells empty', async () => {
	renderRows()

	expect(await screen.findByText('Ana Lopez')).toBeInTheDocument()
	expect(screen.getByText('Skipped')).toBeInTheDocument()
})

test('an outcome the interface does not know reads as the server named it', async () => {
	renderRows([{ ...maria, outcome: 'quarantined' }])

	expect(await screen.findByText('quarantined')).toBeInTheDocument()
})

test('shows the rows in the order the file holds them', async () => {
	renderRows([lucia, maria, ana])

	expect(await shownNames()).toEqual(['Maria Perez', 'Ana Lopez', 'Lucia Gomez'])
	expect(screen.getByRole('columnheader', { name: /Row/ })).toHaveAttribute('aria-sort', 'ascending')
})

test('sorts the rows by a column of the file from its header', async () => {
	renderRows([maria, ana, lucia])
	await screen.findByRole('table')

	await userEvent.click(screen.getByRole('button', { name: 'Name' }))
	await userEvent.click(await screen.findByRole('menuitemradio', { name: 'Sort descending' }))

	await waitFor(async () => expect(await shownNames()).toEqual(['Maria Perez', 'Lucia Gomez', 'Ana Lopez']))
})

test('searches the rows by the text of any cell', async () => {
	renderRows([maria, ana, lucia])

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search rows…' }), 'lucia@')

	await waitFor(async () => expect(await shownNames()).toEqual(['Lucia Gomez']))
})

test('searches the rows by the reason a row carries', async () => {
	renderRows([maria, ana, lucia])

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search rows…' }), 'quote')

	await waitFor(async () => expect(await shownNames()).toEqual(['Lucia Gomez']))
})

test('offers to filter the rows by their outcome alone, by its label', async () => {
	renderRows([maria, ana, lucia])

	await userEvent.click(await screen.findByRole('button', { name: 'Add filter' }))
	const offered = (await screen.findAllByRole('menuitem')).map((item) => item.textContent)
	expect(offered).toEqual(['Outcome'])
	await userEvent.click(screen.getByRole('menuitem', { name: 'Outcome' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Skipped' }))

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez']))
})

test('pages the rows by the page size the admin settings name', async () => {
	paging([2, 4], 2)
	renderRows([maria, ana, lucia])

	await waitFor(async () => expect(await shownNames()).toEqual(['Maria Perez', 'Ana Lopez']))
	await userEvent.click(screen.getByRole('button', { name: 'Next page' }))

	await waitFor(async () => expect(await shownNames()).toEqual(['Lucia Gomez']))
})

test('shows every row on one page when the page size could not be read', async () => {
	server.use(
		graphql.query('AdminSettings', () => HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })),
	)
	renderRows([maria, ana, lucia])

	await waitFor(async () => expect(await shownNames()).toEqual(['Maria Perez', 'Ana Lopez', 'Lucia Gomez']))
	expect(screen.queryByRole('button', { name: 'Next page' })).not.toBeInTheDocument()
})

test('offers as page sizes the ones the admin settings name', async () => {
	paging([2, 4], 2)
	renderRows([maria, ana, lucia])

	await screen.findByRole('table')
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))

	const sizes = within(await screen.findByRole('radiogroup', { name: 'Items per page' }))
	expect(sizes.getAllByRole('radio').map((size) => size.textContent)).toEqual(['2', '4'])
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import type { StoredImport } from '../ImportScreen'
import RowsTable from '../RowsTable'

const rows: StoredImport['rows'] = [
	{
		id: '019f5a00-0000-7000-8000-000000000101',
		position: 1,
		cells: ['Maria Perez', 'maria@example.com'],
		outcome: 'imported',
		reason: null,
	},
	{
		id: '019f5a00-0000-7000-8000-000000000102',
		position: 2,
		cells: ['Ana Lopez'],
		outcome: 'skipped',
		reason: 'the contact detail already belongs to Ana Lopez',
	},
]

const stored: StoredImport = {
	id: '019f5a00-0000-7000-8000-000000000001',
	filename: 'contacts.csv',
	state: 'ready',
	columns: ['Name', ''],
	mapping: [],
	rows,
}

test('the preview shows every cell beside its outcome and reason', async () => {
	render(<RowsTable stored={stored} rows={rows} />)

	expect(await screen.findByText('Maria Perez')).toBeInTheDocument()
	expect(screen.getByText('maria@example.com')).toBeInTheDocument()
	expect(screen.getByText('Imported')).toBeInTheDocument()
	expect(screen.getByText('the contact detail already belongs to Ana Lopez')).toBeInTheDocument()
})

test('the preview adds no first level heading to the screen it sits in', async () => {
	render(<RowsTable stored={stored} rows={rows} />)

	await screen.findByText('Maria Perez')
	expect(
		screen.queryAllByRole('heading').filter((heading) => heading.tagName === 'H1'),
	).toHaveLength(0)
})

test('a blank header is named after its position', async () => {
	render(<RowsTable stored={stored} rows={rows} />)

	expect(await screen.findByText('Column 2')).toBeInTheDocument()
})

test('a row shorter than the header leaves its missing cells empty', async () => {
	render(<RowsTable stored={stored} rows={rows} />)

	expect(await screen.findByText('Ana Lopez')).toBeInTheDocument()
	expect(screen.getByText('Skipped')).toBeInTheDocument()
})

test('an outcome the interface does not know reads as the server named it', async () => {
	const unknown: StoredImport['rows'] = [{ ...rows[0], outcome: 'quarantined' }]
	render(<RowsTable stored={stored} rows={unknown} />)

	expect(await screen.findByText('quarantined')).toBeInTheDocument()
})

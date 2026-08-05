// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import RowsTable from '../RowsTable'
import type { ImportDetail, ImportRow } from '../api'

const stored: ImportDetail = {
	id: '019f5a00-0000-7000-8000-000000000001',
	user_id: '019f5a00-0000-7000-8000-0000000000aa',
	filename: 'contacts.csv',
	state: 'ready',
	row_count: 2,
	imported_count: 0,
	skipped_count: 0,
	failed_count: 0,
	created_at: new Date('2026-08-01T10:00:00Z'),
	columns: ['Name', ''],
	mapping: {},
}

const rows: ImportRow[] = [
	{
		id: '019f5a00-0000-7000-8000-000000000101',
		position: 1,
		cells: ['Maria Perez', 'maria@example.com'],
		outcome: 'imported',
		reason: null,
		contact_id: '019f5a00-0000-7000-8000-0000000000c1',
	},
	{
		id: '019f5a00-0000-7000-8000-000000000102',
		position: 2,
		cells: ['Ana Lopez'],
		outcome: 'skipped',
		reason: 'the contact detail already belongs to Ana Lopez',
		contact_id: null,
	},
]

test('the preview shows every cell beside its outcome and reason', async () => {
	render(<RowsTable stored={stored} rows={rows} />)

	expect(await screen.findByText('Maria Perez')).toBeInTheDocument()
	expect(screen.getByText('maria@example.com')).toBeInTheDocument()
	expect(screen.getByText('imported')).toBeInTheDocument()
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
	expect(screen.getByText('skipped')).toBeInTheDocument()
})

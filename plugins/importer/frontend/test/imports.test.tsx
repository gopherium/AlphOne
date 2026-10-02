// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberFormatLocale } from '@alphone/frontend-sdk'
import {
	http,
	HttpResponse,
	badgeClasses,
	buttonClasses,
	graphql,
	paging,
	server,
	setViewport,
} from '@alphone/frontend-sdk/testing'
import { createRootRoute } from '@tanstack/react-router'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, expect, test } from 'vitest'

import { handlers, importID, multipartBody } from '../handlers'
import { importerIcon } from '../icon'
import { uploadChosen } from '../UploadButton'
import { routes } from '../routes'
import { plugin } from '../index'
import { inSpanish, renderAt } from './harness'

/** stored is one import as the history answers it. */
interface stored {
	__typename: 'ImportJob'
	id: string
	filename: string
	state: string
	rowCount: number
	importedCount: number
	skippedCount: number
	failedCount: number
	createdAt: string
}

/**
 * Builds one import the history answers.
 * @param id - The last hex digits of its identifier.
 * @param filename - The file it was read from.
 * @param changes - The fields that differ from a fresh import of two rows.
 * @returns The import.
 */
function importOf(id: string, filename: string, changes: Partial<stored> = {}): stored {
	return {
		__typename: 'ImportJob',
		id: `019f5a00-0000-7000-8000-0000000000${id}`,
		filename,
		state: 'ready',
		rowCount: 2,
		importedCount: 0,
		skippedCount: 0,
		failedCount: 0,
		createdAt: '2026-08-01T10:00:00Z',
		...changes,
	}
}

const contacts = importOf('01', 'contacts.csv', { id: importID })
const leads = importOf('02', 'leads.csv', { state: 'committing', rowCount: 15, createdAt: '2026-08-02T16:30:00Z' })
const customers = importOf('03', 'customers.xlsx', {
	state: 'committed',
	rowCount: 1234,
	importedCount: 1200,
	skippedCount: 30,
	failedCount: 4,
	createdAt: '2026-08-03T09:05:00Z',
})

/** held is the history the imports query answers, grown by the uploads. */
let held: stored[] = []

/**
 * Serves the given imports from the history.
 * @param imports - The imports the history holds.
 */
function holding(imports: stored[]) {
	held = imports
	server.use(graphql.query('Imports', () => HttpResponse.json({ data: { imports: held } })))
}

/**
 * Answers every upload with the given body, counting the uploads.
 * @param body - The body every upload answers.
 * @returns How many uploads arrived so far.
 */
function uploads(body: (count: number) => Record<string, unknown>) {
	let count = 0
	server.use(
		http.post('/api/graphql', async ({ request }) => {
			if (!(await multipartBody(request)).includes('ImportUpload')) {
				return undefined
			}
			count++
			return HttpResponse.json(body(count))
		}),
	)
	return () => count
}

/**
 * Accepts every upload as a fresh import that joins the history.
 * @returns How many uploads arrived so far.
 */
function accepting() {
	const fresh = importOf('04', 'uploaded.csv', { rowCount: 1, createdAt: '2026-08-04T08:00:00Z' })
	return uploads(() => {
		held = [...held, fresh]
		return { data: { importUpload: { __typename: 'ImportJob', id: fresh.id, filename: fresh.filename } } }
	})
}

/**
 * Returns the table row holding the given text.
 * @param text - A file name the row shows.
 * @returns The row queries.
 */
async function rowOf(text: string) {
	return within(await screen.findByRole('row', { name: new RegExp(text) }))
}

/**
 * Returns the file names the list shows, in order.
 * @returns The names.
 */
async function shownFiles() {
	await screen.findByRole('table')
	return screen
		.getAllByRole('row')
		.slice(1)
		.map((row) => within(row).queryAllByRole('link')[0]?.textContent)
}

/**
 * Returns the file input the Upload button opens.
 * @returns The input.
 */
function fileInput(): HTMLInputElement {
	return screen.getByLabelText('Contacts file')
}

/**
 * Chooses a file through the Upload button.
 * @param file - The file to choose.
 */
async function choose(file: File) {
	await userEvent.click(await screen.findByRole('button', { name: 'Upload' }))
	await userEvent.upload(fileInput(), file)
}

/**
 * Returns the drawing of the icon an empty state holding the given title shows.
 * @param title - The title of the empty state.
 * @returns The path data of its icon.
 */
async function emptyDrawing(title: string): Promise<string | null | undefined> {
	const empty = (await screen.findByText(title)).closest('.godmin-empty')
	return empty?.querySelector('svg path')?.getAttribute('d')
}

/**
 * Returns the drawing the importer icon carries.
 * @returns The path data of its drawing.
 */
function importerDrawing(): string | null | undefined {
	const { container, unmount } = render(importerIcon)
	const drawn = container.querySelector('path')?.getAttribute('d')
	unmount()
	return drawn
}

const csv = new File(['Name,Email\nMaria Perez,maria@example.com\n'], 'uploaded.csv', { type: 'text/csv' })

/** refused is the answer to an upload the server would not read. */
const refused = {
	data: null,
	errors: [{ message: 'the file is neither a CSV nor an Excel workbook', extensions: { code: 'VALIDATION' } }],
}

beforeAll(async () => {
	await import('../ImportsScreen')
})

beforeEach(() => {
	server.use(...handlers)
	holding([leads, contacts, customers])
})

test('keeps in the address only what a list view can hold', () => {
	const [list] = routes(createRootRoute())
	const { validateSearch } = list.options as { validateSearch: (raw: Record<string, unknown>) => unknown }

	expect(validateSearch({ search: 'leads', page: 'abc', order: 'up', tab: 'other' })).toEqual({ search: 'leads' })
})

test('loads the imports list in a chunk of its own, off the first paint', () => {
	const [list] = routes(createRootRoute())

	expect(typeof (list.options.component as { preload?: unknown }).preload).toBe('function')
})

test('keeps the list toolbar while the imports arrive', async () => {
	server.use(graphql.query('Imports', () => new Promise(() => {})))
	renderAt('/import')

	expect(await screen.findByRole('searchbox', { name: 'Search imports…' })).toBeInTheDocument()
	expect(screen.queryByRole('row')).not.toBeInTheDocument()
})

test('says under the title what the import page is for', async () => {
	renderAt('/import')

	const subtitle = await screen.findByText('Bring contacts in from CSV and Excel files.')
	expect(subtitle).toHaveClass('godmin-page__subtitle')
	expect(screen.getByRole('heading', { level: 1, name: 'Import' })).toBeInTheDocument()
})

test('heads the page with the name the menu gives the section, in Spanish too', async () => {
	await inSpanish()
	renderAt('/import')

	const heading = await screen.findByRole('heading', { level: 1 })
	expect([heading.textContent, plugin.nav[0].label]).toEqual(['Importar', 'Importar'])
})

test('draws Upload as a compact button, as a WordPress page header does', async () => {
	renderAt('/import')

	const upload = await screen.findByRole('button', { name: 'Upload' })
	expect([...upload.classList]).toEqual(buttonClasses('solid', 'compact'))
	expect(upload.closest('header')).not.toBeNull()
})

test('lines the list up with the title, out to the canvas edges', async () => {
	renderAt('/import')

	const search = await screen.findByRole('searchbox', { name: 'Search imports…' })
	expect(search.closest('.godmin-page__list')).not.toBeNull()
	expect(search.closest('.dataviews-wrapper')).not.toBeNull()
})

test('heads the columns with the file first, its counts and its start after it', async () => {
	renderAt('/import')

	await rowOf('contacts.csv')
	const headers = screen.getAllByRole('columnheader').map((header) => header.textContent?.replace('↓', ''))
	expect(headers).toEqual(['File', 'State', 'Rows', 'Imported', 'Skipped', 'Failed', 'Started'])
})

test('lines every count up on the right, as WordPress sets numbers', async () => {
	renderAt('/import')

	await rowOf('contacts.csv')
	for (const name of ['Rows', 'Imported', 'Skipped', 'Failed']) {
		expect(screen.getByRole('columnheader', { name })).toHaveStyle({ textAlign: 'end' })
	}
})

test('links every file name to its import', async () => {
	renderAt('/import')

	expect(await screen.findByRole('link', { name: 'contacts.csv' })).toHaveAttribute('href', `/import/${importID}`)
})

test('reads each state as a badge by its label, never by the value the server stores', async () => {
	renderAt('/import')

	expect([...(await rowOf('contacts.csv')).getByText('Ready').classList]).toEqual(badgeClasses('informational'))
	expect([...(await rowOf('leads.csv')).getByText('Importing').classList]).toEqual(badgeClasses('low'))
	expect([...(await rowOf('customers.xlsx')).getByText('Imported').classList]).toEqual(badgeClasses('stable'))
	expect(screen.queryByText('committed')).not.toBeInTheDocument()
	expect(screen.queryByText('committing')).not.toBeInTheDocument()
})

test('reads a state the interface does not know as the server named it, in the outline badge', async () => {
	holding([importOf('05', 'archived.csv', { state: 'archived' })])
	renderAt('/import')

	expect([...(await rowOf('archived.csv')).getByText('archived').classList]).toEqual(badgeClasses('none'))
})

test('writes its counts in the format locale and its start as a day and a time', async () => {
	renderAt('/import')

	const cells = (await rowOf('customers.xlsx')).getAllByRole('cell').map((cell) => cell.textContent)
	expect(cells).toEqual(['customers.xlsx', 'Imported', '1.234', '1.200', '30', '4', '03/08/2026 09:05'])
})

test('asks for the imports again on every visit, so the counts of a commit made meanwhile show on return', async () => {
	const router = renderAt('/import')
	await rowOf('contacts.csv')
	holding([leads, { ...contacts, state: 'committed', importedCount: 1, skippedCount: 1 }, customers])

	await act(() => router.navigate({ to: '/' }))
	await act(() => router.navigate({ to: '/import' }))

	await waitFor(async () =>
		expect((await rowOf('contacts.csv')).getAllByRole('cell').map((cell) => cell.textContent)).toEqual([
			'contacts.csv',
			'Imported',
			'2',
			'1',
			'1',
			'0',
			'01/08/2026 10:00',
		]),
	)
})

test('writes its counts in the digits the format locale names', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	renderAt('/import')

	expect((await rowOf('customers.xlsx')).getByText('१.२३४')).toBeInTheDocument()
})

test('sorts the imports by when they started, newest first, when the address names no order', async () => {
	renderAt('/import')

	expect(await shownFiles()).toEqual(['customers.xlsx', 'leads.csv', 'contacts.csv'])
	expect(screen.getByRole('columnheader', { name: /Started/ })).toHaveAttribute('aria-sort', 'descending')
})

test.each(['rowCount', 'importedCount', 'skippedCount', 'failedCount'])(
	'sorts the imports by the %s column the address names, the biggest first',
	async (field) => {
		renderAt(`/import?sort=${field}&order=desc`)

		expect((await shownFiles())[0]).toBe('customers.xlsx')
	},
)

test('sorts the imports by how many rows they hold, fewest first, when the address says so', async () => {
	renderAt('/import?sort=rowCount&order=asc')

	expect(await shownFiles()).toEqual(['contacts.csv', 'leads.csv', 'customers.xlsx'])
})

test('searches the imports by file name', async () => {
	renderAt('/import')

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search imports…' }), 'leads')

	await waitFor(async () => expect(await shownFiles()).toEqual(['leads.csv']))
})

test('narrows the imports to the states the address names', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'state', operator: 'isAny', value: ['committed'] }]))
	renderAt(`/import?filters=${filters}`)

	expect(await shownFiles()).toEqual(['customers.xlsx'])
})

test('keeps the filter button live, with no filter chip until a filter is picked', async () => {
	renderAt('/import')

	await screen.findByRole('table')
	expect(screen.getByRole('button', { name: 'Add filter' })).not.toHaveAttribute('aria-disabled', 'true')
	expect(screen.getAllByRole('button', { name: 'State' })).toHaveLength(1)
})

test('narrows the imports to the state picked through the filter button, offering states by label', async () => {
	renderAt('/import')

	await userEvent.click(await screen.findByRole('button', { name: 'Add filter' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: 'State' }))
	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Ready', 'Importing', 'Imported'])
	await userEvent.click(screen.getByRole('option', { name: 'Ready' }))

	await waitFor(async () => expect(await shownFiles()).toEqual(['contacts.csv']))
})

test('counts nothing and ticks nothing, because the list offers no bulk action', async () => {
	renderAt('/import')

	await rowOf('contacts.csv')
	expect(screen.queryByText(/Items$/)).not.toBeInTheDocument()
	expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
})

test('pages the imports by the page size the admin settings name', async () => {
	paging([2, 4], 2)
	renderAt('/import')

	expect(await shownFiles()).toEqual(['customers.xlsx', 'leads.csv'])
	await userEvent.click(screen.getByRole('button', { name: 'Next page' }))

	await waitFor(async () => expect(await shownFiles()).toEqual(['contacts.csv']))
})

test('offers as page sizes the ones the admin settings name', async () => {
	paging([2, 4], 2)
	renderAt('/import')

	await screen.findByRole('table')
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))

	const sizes = within(await screen.findByRole('radiogroup', { name: 'Items per page' }))
	expect(sizes.getAllByRole('radio').map((size) => size.textContent)).toEqual(['2', '4'])
})

test('shows every import on one page when the page size could not be read', async () => {
	server.use(
		graphql.query('AdminSettings', () => HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })),
	)
	renderAt('/import')

	await waitFor(async () => expect(await shownFiles()).toEqual(['customers.xlsx', 'leads.csv', 'contacts.csv']))
	expect(screen.queryByRole('button', { name: 'Next page' })).not.toBeInTheDocument()
})

test('lays the imports out as a list on a phone, each file still a link to its import', async () => {
	setViewport({ matches: true })
	renderAt('/import')

	expect(await screen.findByRole('link', { name: 'contacts.csv' })).toHaveAttribute('href', `/import/${importID}`)
	expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

test('the list a phone shows carries the state and the start of each import, never a bare count', async () => {
	setViewport({ matches: true })
	renderAt('/import')

	await screen.findByRole('button', { name: 'customers.xlsx' })
	const labels = [...document.querySelectorAll('.dataviews-view-list__field-label')].map((label) => label.textContent)
	expect(new Set(labels)).toEqual(new Set(['State', 'Started']))
})

test('opens the import a tap lands on in the list a phone shows', async () => {
	setViewport({ matches: true })
	const router = renderAt('/import')

	await userEvent.click(await screen.findByRole('button', { name: 'contacts.csv' }))

	await waitFor(() => expect(router.state.location.pathname).toBe(`/import/${importID}`))
})

test('reports when the imports cannot be loaded', async () => {
	server.use(graphql.query('Imports', () => HttpResponse.json({ data: null, errors: [{ message: 'nope' }] })))
	renderAt('/import')

	expect(await screen.findByRole('alert')).toHaveTextContent('Imports could not be loaded.')
})

test('shows an empty state with the import icon when no import exists', async () => {
	holding([])
	renderAt('/import')

	expect(await screen.findByText('No imports yet.')).toBeInTheDocument()
	expect(screen.getByText('Upload a CSV or Excel file to start one.').closest('.godmin-empty')).not.toBeNull()
	expect(await emptyDrawing('No imports yet.')).toBe(importerDrawing())
})

test('says no import matched when a search finds none', async () => {
	renderAt('/import?search=nothing')

	expect(await screen.findByText('No imports found.')).toBeInTheDocument()
	expect(screen.queryByText('No imports yet.')).not.toBeInTheDocument()
	expect(await emptyDrawing('No imports found.')).toBe(importerDrawing())
})

test('opens the file dialog from the Upload button, for CSV and Excel files only', async () => {
	renderAt('/import')
	let opened = 0

	const upload = await screen.findByRole('button', { name: 'Upload' })
	fileInput().addEventListener('click', () => opened++)
	await userEvent.click(upload)

	expect(opened).toBe(1)
	expect(fileInput()).toHaveAttribute('accept', '.csv,.xlsx')
})

test('uploads the chosen file, confirms it with a toast and refreshes the list', async () => {
	const count = accepting()
	renderAt('/import')
	await rowOf('contacts.csv')

	await choose(csv)

	expect(await screen.findByText('File uploaded.')).toBeInTheDocument()
	expect(await screen.findByRole('link', { name: 'uploaded.csv' })).toBeInTheDocument()
	expect(count()).toBe(1)
})

test('opens the uploaded import from the toast, ready to map', async () => {
	accepting()
	const router = renderAt('/import')
	await rowOf('contacts.csv')

	await choose(csv)
	await userEvent.click(await screen.findByRole('button', { name: 'Open' }))

	await waitFor(() => expect(router.state.location.pathname).toBe('/import/019f5a00-0000-7000-8000-000000000004'))
	expect(await screen.findByRole('button', { name: 'Save mapping' })).toBeInTheDocument()
})

test('lets the same file be chosen again once an upload started', async () => {
	accepting()
	renderAt('/import')
	await rowOf('contacts.csv')

	await choose(csv)

	await screen.findByText('File uploaded.')
	expect(fileInput().value).toBe('')
})

test('reports a refused upload above the list, without a toast', async () => {
	uploads(() => refused)
	renderAt('/import')
	await rowOf('contacts.csv')

	await choose(new File(['nope'], 'notes.csv', { type: 'text/csv' }))

	const alert = await screen.findByRole('alert')
	expect(alert).toHaveTextContent('the file is neither a CSV nor an Excel workbook')
	expect(alert.compareDocumentPosition(screen.getByRole('table')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
	expect(screen.queryByText('File uploaded.')).not.toBeInTheDocument()
})

test('clears an earlier upload failure once the next upload starts', async () => {
	const fresh = importOf('04', 'uploaded.csv')
	uploads((count) =>
		count === 1
			? refused
			: { data: { importUpload: { __typename: 'ImportJob', id: fresh.id, filename: fresh.filename } } },
	)
	renderAt('/import')
	await rowOf('contacts.csv')
	await choose(new File(['nope'], 'notes.csv', { type: 'text/csv' }))
	expect(await screen.findByRole('alert')).toBeInTheDocument()

	await choose(csv)

	expect(await screen.findByText('File uploaded.')).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('cancelling the file dialog uploads nothing', () => {
	let count = 0
	const upload = () => count++

	uploadChosen(null, upload)
	uploadChosen([] as unknown as FileList, upload)

	expect(count).toBe(0)
})

test('choosing a file hands it to the upload', () => {
	const chosen: File[] = []
	const file = new File(['Name\n'], 'contacts.csv')

	uploadChosen([file] as unknown as FileList, (picked) => chosen.push(picked))

	expect(chosen).toEqual([file])
})

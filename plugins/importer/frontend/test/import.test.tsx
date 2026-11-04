// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server, textClasses } from '@alphone/frontend-sdk/testing'
import { createRootRoute } from '@tanstack/react-router'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, expect, test } from 'vitest'

import { handlers, importID } from '../handlers'
import { plugin } from '../index'
import { routes } from '../routes'
import { inSpanish, renderAt } from './harness'

/**
 * Renders the import screen for the fixture import at its own address, below a toaster.
 * @returns The router, to read where a test ended up.
 */
function renderScreen() {
	return renderAt(`/import/${importID}`)
}

/**
 * Returns the breadcrumb that leads to the list of imports.
 * @returns The link.
 */
async function importBreadcrumb() {
	const breadcrumbs = await screen.findByRole('navigation', { name: 'Breadcrumbs' })
	return within(breadcrumbs).getByRole('link', { name: 'Import' })
}

/**
 * Serves the detail document for an import with the given fields.
 * @param job - The import job fields overriding the ready defaults.
 */
function detailOf(job: Record<string, unknown> = {}) {
	server.use(
		graphql.query('ImportDetail', () =>
			HttpResponse.json({
				data: {
					importJob: {
						__typename: 'ImportJob',
						id: importID,
						filename: 'contacts.csv',
						state: 'ready',
						columns: ['Name', 'Email'],
						mapping: [],
						...job,
					},
					importFields: importFields,
				},
			}),
		),
	)
}

const importFields = [
	{ __typename: 'ImportField', name: 'name', label: 'Name', required: true },
	{ __typename: 'ImportField', name: 'email', label: 'Email', required: false },
	{ __typename: 'ImportField', name: 'phone', label: 'Phone', required: false },
]

/** refusedMappingToast is what a mapping the server refuses raises. */
const refusedMappingToast = 'Could not save the mapping. no assignment claims the required field "name"'

/** refusedCommitToast is what a commit the server refuses raises. */
const refusedCommitToast = 'Could not commit the import. the import carries no mapping yet'

/**
 * Renders the screen and waits until its mapping form has settled.
 */
async function renderSettled() {
	renderScreen()
	await screen.findByRole('button', { name: 'Save mapping' })
}

/**
 * Chooses a field for the named column.
 * @param column - The column label.
 * @param field - The field label to choose.
 */
async function chooseField(column: string, field: string) {
	await userEvent.click(await screen.findByRole('combobox', { name: column }))
	await userEvent.click(await screen.findByRole('option', { name: field }))
	await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
}

beforeAll(() => import('../RowsTable'))

beforeEach(() => {
	server.use(...handlers)
})

test('loads the import screen in a chunk of its own, off the first paint', () => {
	const [, detail] = routes(createRootRoute())

	expect(typeof (detail.options.component as { preload?: unknown }).preload).toBe('function')
})

test('keeps the page chrome while the import is on its way', async () => {
	server.use(graphql.query('ImportDetail', () => new Promise(() => {})))
	renderScreen()

	expect(await screen.findByRole('heading', { level: 2, name: 'Import' })).toBeInTheDocument()
	const status = screen.getByRole('status')
	expect(status).toHaveTextContent('Loading import…')
	expect(status.closest('.godmin-loading-screen')).not.toBeNull()
	expect(await importBreadcrumb()).toHaveAttribute('href', '/import')
})

test('heads the loading page with the name the menu gives the section, in Spanish too', async () => {
	await inSpanish()
	server.use(graphql.query('ImportDetail', () => new Promise(() => {})))
	renderScreen()

	const heading = await screen.findByRole('heading', { level: 2 })
	expect([heading.textContent, plugin.nav[0].label]).toEqual(['Importar', 'Importar'])
})

test('the screen names the file it is mapping', async () => {
	renderScreen()

	expect(await screen.findByRole('heading', { name: 'contacts.csv' })).toBeInTheDocument()
})

test('the breadcrumb leads back to the list of imports', async () => {
	const router = renderScreen()
	await screen.findByRole('heading', { name: 'contacts.csv' })
	const imports = await importBreadcrumb()

	expect(imports).toHaveAttribute('href', '/import')
	expect(imports).not.toHaveAttribute('aria-current')
	await userEvent.click(imports)

	await waitFor(() => expect(router.state.location.pathname).toBe('/import'))
	expect(await screen.findByRole('heading', { level: 2, name: 'Import' })).toBeInTheDocument()
})

test('the screen sets the Rows heading a size above the field labels', async () => {
	renderScreen()

	const heading = await screen.findByRole('heading', { level: 3, name: 'Rows' })
	expect([...heading.classList]).toEqual(expect.arrayContaining(textClasses('heading-lg')))
})

test(
	'a chosen field is saved against its column',
	async () => {
		let saved: unknown = null
		server.use(
			graphql.mutation('ImportSetMapping', ({ variables }) => {
				saved = variables.assignments
				return HttpResponse.json({
					data: {
						importSetMapping: {
							__typename: 'ImportJob',
							id: importID,
							state: 'ready',
							mapping: [{ __typename: 'ImportAssignment', column: 0, field: 'name' }],
						},
					},
				})
			}),
		)
		await renderSettled()

		await chooseField('Name', 'Name')
		await userEvent.click(await screen.findByRole('button', { name: 'Save mapping' }))

		await waitFor(() => expect(saved).toEqual([{ column: 0, field: 'name' }]))
		expect(await screen.findByText('Mapping saved.')).toBeInTheDocument()
	},
	20000,
)

test('the commit turns the rows into contacts and says what it did with them', async () => {
	let committed: unknown = null
	server.use(
		graphql.mutation('ImportCommit', ({ variables }) => {
			committed = variables.id
			return HttpResponse.json({
				data: {
					importCommit: {
						__typename: 'ImportCommitPayload',
						id: importID,
						imported: 1234,
						skipped: 1,
						failed: 0,
					},
				},
			})
		}),
	)
	await renderSettled()

	await userEvent.click(await screen.findByRole('button', { name: 'Commit' }))

	await waitFor(() => expect(committed).toBe(importID))
	expect(await screen.findByText('Import finished: 1.234 imported, 1 skipped, 0 failed.')).toBeInTheDocument()
})

/**
 * Serves an error for the named mutation.
 * @param operation - The mutation the server refuses.
 * @param message - The reason the server gives.
 */
function refusing(operation: string, message: string) {
	server.use(
		graphql.mutation(operation, () =>
			HttpResponse.json({ data: null, errors: [{ message, extensions: { code: 'VALIDATION' } }] }),
		),
	)
}

/** writes are the two writes the mapping form runs, with how each is told apart. */
const writes = [
	{
		operation: 'ImportSetMapping',
		button: 'Save mapping',
		failed: 'Could not save the mapping.',
		words: 'no assignment claims the required field "name"',
		worked: /^Mapping saved/,
	},
	{
		operation: 'ImportCommit',
		button: 'Commit',
		failed: 'Could not commit the import.',
		words: 'the import carries no mapping yet',
		worked: /^Import finished/,
	},
]

test.each(writes)('$button says in a toast that the server refused it, in its words', async (write) => {
	refusing(write.operation, write.words)
	await renderSettled()

	await userEvent.click(await screen.findByRole('button', { name: write.button }))

	expect(await screen.findByText(`${write.failed} ${write.words}`, { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.queryByText(write.worked)).not.toBeInTheDocument()
})

test.each(writes)('$button says only what failed with no words from the server, and reads again', async (write) => {
	server.use(
		graphql.mutation(write.operation, () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	await renderSettled()
	detailOf({ filename: 'read-again.csv' })

	await userEvent.click(await screen.findByRole('button', { name: write.button }))

	expect(await screen.findByText(write.failed, { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(await screen.findByRole('heading', { name: 'read-again.csv' })).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test(
	'a refused mapping keeps the chosen field once the import is read again, both buttons still on',
	async () => {
		refusing('ImportSetMapping', 'no assignment claims the required field "name"')
		await renderSettled()
		await chooseField('Email', 'Phone')
		detailOf({ filename: 'read-again.csv' })

		await userEvent.click(screen.getByRole('button', { name: 'Save mapping' }))

		await screen.findByText(refusedMappingToast, { selector: '.godmin-toast' })
		await screen.findByRole('heading', { name: 'read-again.csv' })
		expect(screen.getByRole('combobox', { name: 'Email' })).toHaveTextContent('Phone')
		expect(screen.getByRole('button', { name: 'Save mapping' })).not.toHaveAttribute('aria-disabled', 'true')
		expect(screen.getByRole('button', { name: 'Commit' })).not.toHaveAttribute('aria-disabled', 'true')
	},
	20000,
)

test(
	'a mapping still saves after a refused commit, the chosen field kept',
	async () => {
		let saved: unknown = null
		refusing('ImportCommit', 'the import carries no mapping yet')
		server.use(
			graphql.mutation('ImportSetMapping', ({ variables }) => {
				saved = variables.assignments
				return HttpResponse.json({
					data: {
						importSetMapping: {
							__typename: 'ImportJob',
							id: importID,
							state: 'ready',
							mapping: [{ __typename: 'ImportAssignment', column: 1, field: 'phone' }],
						},
					},
				})
			}),
		)
		await renderSettled()
		await chooseField('Email', 'Phone')
		detailOf({ filename: 'read-again.csv' })
		await userEvent.click(screen.getByRole('button', { name: 'Commit' }))
		await screen.findByText(refusedCommitToast, { selector: '.godmin-toast' })
		await screen.findByRole('heading', { name: 'read-again.csv' })
		expect(screen.getByRole('combobox', { name: 'Email' })).toHaveTextContent('Phone')

		await userEvent.click(screen.getByRole('button', { name: 'Save mapping' }))

		expect(await screen.findByText('Mapping saved.')).toBeInTheDocument()
		expect(saved).toEqual([{ column: 1, field: 'phone' }])
	},
	20000,
)

test.each(writes)(
	'$button keeps the form and the chosen field when the server gives no answer at all',
	async (write) => {
		server.use(graphql.mutation(write.operation, () => HttpResponse.error()))
		await renderSettled()
		await chooseField('Email', 'Phone')
		server.use(graphql.query('ImportDetail', () => HttpResponse.error()))

		await userEvent.click(screen.getByRole('button', { name: write.button }))

		await screen.findByText(write.failed, { selector: '.godmin-toast' })
		await new Promise((resolve) => setTimeout(resolve, 50))
		expect(screen.queryByRole('alert')).not.toBeInTheDocument()
		expect(screen.getByRole('combobox', { name: 'Email' })).toHaveTextContent('Phone')
		expect(screen.getByRole('button', { name: write.button })).not.toHaveAttribute('aria-disabled', 'true')
	},
	20000,
)

test('a mapping on its way turns Save mapping off', async () => {
	server.use(graphql.mutation('ImportSetMapping', () => new Promise(() => {})))
	await renderSettled()

	await userEvent.click(await screen.findByRole('button', { name: 'Save mapping' }))

	await waitFor(() =>
		expect(screen.getByRole('button', { name: 'Save mapping' })).toHaveAttribute('aria-disabled', 'true'),
	)
})

test('a commit on its way turns Commit off', async () => {
	server.use(graphql.mutation('ImportCommit', () => new Promise(() => {})))
	await renderSettled()

	await userEvent.click(await screen.findByRole('button', { name: 'Commit' }))

	await waitFor(() => expect(screen.getByRole('button', { name: 'Commit' })).toHaveAttribute('aria-disabled', 'true'))
})

test('a committed import accepts neither a mapping nor another commit', async () => {
	detailOf({
		filename: 'done.csv',
		state: 'committed',
		columns: ['Name'],
		mapping: [{ __typename: 'ImportAssignment', column: 0, field: 'name' }],
	})
	renderScreen()
	await screen.findByRole('heading', { name: 'done.csv' })

	expect(await screen.findByRole('button', { name: 'Save mapping' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
	expect(await screen.findByRole('button', { name: 'Commit' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('a stored mapping arrives already chosen', async () => {
	server.use(
		graphql.query('ImportDetail', () =>
			HttpResponse.json({
				data: {
					importJob: {
						__typename: 'ImportJob',
						id: importID,
						filename: 'mapped.csv',
						state: 'ready',
						columns: ['Name', ''],
						mapping: [{ __typename: 'ImportAssignment', column: 0, field: 'name' }],
					},
					importFields,
				},
			}),
		),
	)
	renderScreen()

	await screen.findByRole('heading', { name: 'mapped.csv' })

	expect(await screen.findByLabelText('Name')).toHaveTextContent('Name')
	expect(screen.getByLabelText('Column 2')).toHaveTextContent('Not imported')
})

test('offers the contact fields by the names the interface gives them, a defined field by its own label', async () => {
	server.use(
		graphql.query('ImportDetail', () =>
			HttpResponse.json({
				data: {
					importJob: {
						__typename: 'ImportJob',
						id: importID,
						filename: 'contacts.csv',
						state: 'ready',
						columns: ['Name'],
						mapping: [],
					},
					importFields: [
						{ __typename: 'ImportField', name: 'name', label: 'Full name', required: true },
						{ __typename: 'ImportField', name: 'email', label: 'E-mail address', required: false },
						{ __typename: 'ImportField', name: 'phone', label: 'Telephone', required: false },
						{ __typename: 'ImportField', name: 'joinedOn', label: 'Joined on', required: false },
					],
				},
			}),
		),
	)
	await renderSettled()

	await userEvent.click(await screen.findByLabelText('Name'))

	const offered = within(await screen.findByRole('listbox')).getAllByRole('option').map((option) => option.textContent)
	expect(offered).toEqual(['Not imported', 'Name', 'Email', 'Phone', 'Joined on'])
})

test('the rows section shows a spinner until every page of rows is read', async () => {
	server.use(graphql.query('ImportRows', () => new Promise(() => {})))
	renderScreen()

	await screen.findByRole('heading', { name: 'contacts.csv' })

	expect(screen.getByRole('status')).toHaveTextContent('Loading the preview…')
	expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

test('the rows section reports rows it cannot read, keeping the mapping', async () => {
	server.use(graphql.query('ImportRows', () => HttpResponse.json({ data: null, errors: [{ message: 'gone' }] })))
	renderScreen()

	expect(await screen.findByRole('alert')).toHaveTextContent('The import could not be loaded.')
	expect(screen.getByRole('button', { name: 'Save mapping' })).toBeInTheDocument()
})

test('the rows section shows every staged row once they are read', async () => {
	renderScreen()

	const table = await screen.findByRole('table')

	expect(within(table).getByText('Maria Perez')).toBeInTheDocument()
	expect(within(table).getByText('Ana Lopez')).toBeInTheDocument()
})

test('the screen reports an import it cannot read', async () => {
	server.use(
		graphql.query('ImportDetail', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'gone' }] }),
		),
	)

	renderScreen()

	expect(await screen.findByRole('alert')).toHaveTextContent('The import could not be loaded.')
	expect(screen.getByRole('heading', { level: 2, name: 'Import' })).toBeInTheDocument()
	expect(await importBreadcrumb()).toHaveAttribute('href', '/import')
})

test('the screen reports an import that is gone', async () => {
	server.use(
		graphql.query('ImportDetail', () =>
			HttpResponse.json({ data: { importJob: null, importFields } }),
		),
	)

	renderScreen()

	expect(await screen.findByRole('alert')).toHaveTextContent('The import could not be loaded.')
	expect(screen.getByRole('heading', { level: 2, name: 'Import' })).toBeInTheDocument()
	expect(await importBreadcrumb()).toHaveAttribute('href', '/import')
})

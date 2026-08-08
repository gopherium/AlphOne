// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider } from '@alphone/frontend-sdk'
import {
	HttpResponse,
	fakeGraphClient,
	graphql,
	server,
} from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { ImportScreen } from '../ImportScreen'
import { handlers, importID } from '../handlers'

/**
 * Renders the import screen for the fixture import.
 */
function renderScreen() {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	const { graph } = fakeGraphClient()
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<ImportScreen importId={importID} />
			</GraphProvider>
		</QueryClientProvider>,
	)
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
						rows: [],
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
	await userEvent.click(await screen.findByLabelText(column))
	await userEvent.click(await screen.findByRole('option', { name: field }))
	await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
}

beforeEach(() => {
	server.use(...handlers)
})

test('keeps the page chrome while the import is on its way', async () => {
	server.use(graphql.query('ImportDetail', () => new Promise(() => {})))
	renderScreen()

	expect(await screen.findByRole('heading', { level: 1, name: 'Import' })).toBeInTheDocument()
	const status = screen.getByRole('status')
	expect(status).toHaveTextContent('Loading import…')
	expect(status.closest('.godmin-loading-screen')).not.toBeNull()
})

test('the screen names the file it is mapping', async () => {
	renderScreen()

	expect(await screen.findByRole('heading', { name: 'contacts.csv' })).toBeInTheDocument()
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
	},
	20000,
)

test('a refused mapping is reported', async () => {
	server.use(
		graphql.mutation('ImportSetMapping', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'no assignment claims the required field "name"',
						extensions: { code: 'VALIDATION' },
					},
				],
			}),
		),
	)
	await renderSettled()

	await userEvent.click(await screen.findByRole('button', { name: 'Save mapping' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'no assignment claims the required field "name"',
	)
})

test('the commit turns the rows into contacts', async () => {
	let committed: unknown = null
	server.use(
		graphql.mutation('ImportCommit', ({ variables }) => {
			committed = variables.id
			return HttpResponse.json({
				data: {
					importCommit: {
						__typename: 'ImportCommitPayload',
						id: importID,
						imported: 2,
						skipped: 0,
						failed: 0,
					},
				},
			})
		}),
	)
	await renderSettled()

	await userEvent.click(await screen.findByRole('button', { name: 'Commit' }))

	await waitFor(() => expect(committed).toBe(importID))
})

test('a refused commit is reported', async () => {
	server.use(
		graphql.mutation('ImportCommit', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'the import carries no mapping yet',
						extensions: { code: 'VALIDATION' },
					},
				],
			}),
		),
	)
	await renderSettled()

	await userEvent.click(await screen.findByRole('button', { name: 'Commit' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('the import carries no mapping yet')
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
						rows: [],
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

test('the screen reports an import it cannot read', async () => {
	server.use(
		graphql.query('ImportDetail', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'gone' }] }),
		),
	)

	renderScreen()

	expect(await screen.findByRole('alert')).toHaveTextContent('The import could not be loaded.')
})

test('the screen reports an import that is gone', async () => {
	server.use(
		graphql.query('ImportDetail', () =>
			HttpResponse.json({ data: { importJob: null, importFields } }),
		),
	)

	renderScreen()

	expect(await screen.findByRole('alert')).toHaveTextContent('The import could not be loaded.')
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider } from '@alphone/frontend-sdk'
import {
	HttpResponse,
	fakeGraphClient,
	graphql,
	server,
} from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { FieldsScreen } from '../FieldsScreen'

const birthDate = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000501',
	name: 'birthDate',
	label: 'Birth date',
	kind: 'DATE',
}

function renderScreen() {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	const { graph } = fakeGraphClient()
	return render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<FieldsScreen />
			</GraphProvider>
		</QueryClientProvider>,
	)
}

function serveFields(fields: unknown[]) {
	server.use(
		graphql.query('Fields', () => HttpResponse.json({ data: { fields } })),
	)
}

test('the catalogue lists every defined field in a table', async () => {
	serveFields([birthDate])

	renderScreen()

	const table = await screen.findByRole('table')
	const row = within(table).getAllByRole('row')[1]
	const cells = within(row).getAllByRole('cell')
	expect(cells[0]).toHaveTextContent('Birth date')
	expect(cells[1]).toHaveTextContent('birthDate')
	expect(cells[2]).toHaveTextContent('Date')
	expect(within(table).getByRole('columnheader', { name: 'Label' })).toBeInTheDocument()
	expect(within(table).getByRole('columnheader', { name: 'Name' })).toBeInTheDocument()
	expect(within(table).getByRole('columnheader', { name: 'Kind' })).toBeInTheDocument()
})

test('an empty catalogue invites the first field', async () => {
	serveFields([])

	renderScreen()

	expect(await screen.findByText(/No fields yet/i)).toBeInTheDocument()
})

test('a failed read is reported', async () => {
	server.use(
		graphql.query('Fields', () =>
			HttpResponse.json({ errors: [{ message: 'boom' }] }),
		),
	)

	renderScreen()

	expect(await screen.findByRole('alert')).toBeInTheDocument()
})

test('defining a field sends its name, label and kind', async () => {
	serveFields([])
	const defined = vi.fn()
	server.use(
		graphql.mutation('DefineField', async ({ variables }) => {
			defined(variables)
			return HttpResponse.json({ data: { defineField: birthDate } })
		}),
	)

	renderScreen()
	await screen.findByText(/No fields yet/i)
	await userEvent.type(await screen.findByLabelText('Label'), 'Birth date')
	await userEvent.type(screen.getByLabelText('Name'), 'birthDate')
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

	await waitFor(() =>
		expect(defined).toHaveBeenCalledWith({
			name: 'birthDate',
			label: 'Birth date',
			kind: 'TEXT',
		}),
	)
})

async function submitField() {
	await screen.findByText(/No fields yet/i)
	await userEvent.type(await screen.findByLabelText('Label'), 'Birth date')
	await userEvent.type(screen.getByLabelText('Name'), 'birthDate')
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))
}

test('a taken name is reported word for word', async () => {
	serveFields([])
	server.use(
		graphql.mutation('DefineField', () =>
			HttpResponse.json({
				errors: [
					{
						message: 'fields: another definition holds that name',
						extensions: { code: 'CONFLICT' },
					},
				],
			}),
		),
	)

	renderScreen()
	await submitField()

	expect(await screen.findByRole('alert')).toHaveTextContent(/holds that name/)
})

test('the chosen kind is sent with the definition', async () => {
	serveFields([])
	const defined = vi.fn()
	server.use(
		graphql.mutation('DefineField', async ({ variables }) => {
			defined(variables)
			return HttpResponse.json({ data: { defineField: birthDate } })
		}),
	)

	renderScreen()
	await screen.findByText(/No fields yet/i)
	await userEvent.type(await screen.findByLabelText('Label'), 'Birth date')
	await userEvent.type(screen.getByLabelText('Name'), 'birthDate')
	await userEvent.click(screen.getByRole('combobox', { name: 'Kind' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Date' }))
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

	await waitFor(() =>
		expect(defined).toHaveBeenCalledWith({
			name: 'birthDate',
			label: 'Birth date',
			kind: 'DATE',
		}),
	)
})

test('marks the kind the reader chose as the selected option', async () => {
	serveFields([])
	renderScreen()
	await screen.findByText(/No fields yet/i)

	await userEvent.click(screen.getByRole('combobox', { name: 'Kind' }))

	expect(await screen.findByRole('option', { name: 'Text' })).toHaveAttribute(
		'aria-selected',
		'true',
	)
})

test('a defined field appears in the catalogue without a reload', async () => {
	let served: unknown[] = []
	server.use(
		graphql.query('Fields', () => HttpResponse.json({ data: { fields: served } })),
		graphql.mutation('DefineField', () => {
			served = [birthDate]
			return HttpResponse.json({ data: { defineField: birthDate } })
		}),
	)

	renderScreen()
	await submitField()

	expect(await screen.findByRole('table')).toBeInTheDocument()
	expect(screen.getByText('Birth date')).toBeInTheDocument()
})

test('an answer carrying no catalogue reads as empty', async () => {
	server.use(graphql.query('Fields', () => HttpResponse.json({ data: {} })))

	renderScreen()

	expect(await screen.findByText(/No fields yet/i)).toBeInTheDocument()
})

test('a failed archive is reported', async () => {
	serveFields([birthDate])
	server.use(
		graphql.mutation('ArchiveField', () =>
			HttpResponse.json({
				errors: [
					{ message: 'fields: no live definition holds that id', extensions: { code: 'NOT_FOUND' } },
				],
			}),
		),
	)

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Archive Birth date' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The field could not be archived.',
	)
})

test('a validation error is reported word for word', async () => {
	serveFields([])
	server.use(
		graphql.mutation('DefineField', () =>
			HttpResponse.json({
				errors: [
					{
						message: 'fields: a name is camelCase, starting with a lowercase letter',
						extensions: { code: 'VALIDATION' },
					},
				],
			}),
		),
	)

	renderScreen()
	await submitField()

	expect(await screen.findByRole('alert')).toHaveTextContent(/camelCase/)
})

test('archiving a field sends its id', async () => {
	serveFields([birthDate])
	const archived = vi.fn()
	server.use(
		graphql.mutation('ArchiveField', async ({ variables }) => {
			archived(variables)
			return HttpResponse.json({ data: { archiveField: true } })
		}),
	)

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Archive Birth date' }))

	await waitFor(() => expect(archived).toHaveBeenCalledWith({ id: birthDate.id }))
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, configureErrorText } from '@alphone/frontend-sdk'
import {
	HttpResponse,
	fakeGraphClient,
	graphql,
	server,
} from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { FieldsScreen } from '../FieldsScreen'
import { refusal, speakTemplates } from './harness'

afterEach(() => {
	configureErrorText({ templates: () => ({}), fallback: () => '' })
})

const RESERVED = [
	'constructor', 'createdAt', 'field', 'hasOwnProperty', 'id', 'identities', 'isPrototypeOf',
	'name', 'propertyIsEnumerable', 'tasks', 'toLocaleString', 'toString', 'valueOf', 'whatsAppConversations',
]

const birthDate = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000501',
	name: 'birthDate',
	label: 'Birth date',
	kind: 'DATE',
	subFields: [],
}

function renderScreen(graph = fakeGraphClient().graph) {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	return render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<FieldsScreen />
			</GraphProvider>
		</QueryClientProvider>,
	)
}

function serveFieldCatalogue(live: unknown[], archived: unknown[] = [], reserved: string[] = RESERVED) {
	server.use(
		graphql.query('FieldCatalogue', () =>
			HttpResponse.json({ data: { fields: live, every: [...live, ...archived], reservedFieldNames: reserved } }),
		),
	)
}

function captureDefine(answer: unknown = birthDate) {
	const defined = vi.fn()
	server.use(
		graphql.mutation('DefineField', async ({ variables }) => {
			defined(variables)
			return HttpResponse.json({ data: { defineField: answer } })
		}),
	)
	return defined
}

async function defineLabelled(label: string) {
	await userEvent.type(await screen.findByLabelText('Label'), label)
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))
}

async function sentName(defined: ReturnType<typeof vi.fn>) {
	await waitFor(() => expect(defined).toHaveBeenCalledTimes(1))
	return defined.mock.calls[0][0].name
}

test('the catalogue lists every defined field in a table', async () => {
	serveFieldCatalogue([birthDate])

	renderScreen()

	const table = await screen.findByRole('table')
	const row = within(table).getAllByRole('row')[1]
	const cells = within(row).getAllByRole('cell')
	expect(cells[0]).toHaveTextContent('Birth date')
	expect(cells[1]).toHaveTextContent('birthDate')
	expect(cells[2]).toHaveTextContent('Date')
	expect(within(table).getByRole('columnheader', { name: 'Label' })).toBeInTheDocument()
	expect(within(table).getByRole('columnheader', { name: 'API name' })).toBeInTheDocument()
	expect(within(table).getByRole('columnheader', { name: 'Kind' })).toBeInTheDocument()
})

test('an empty catalogue invites the first field', async () => {
	serveFieldCatalogue([])

	renderScreen()

	expect(await screen.findByText(/No fields yet/i)).toBeInTheDocument()
})

test('a failed read is reported', async () => {
	server.use(
		graphql.query('FieldCatalogue', () =>
			HttpResponse.json({ errors: [{ message: 'boom' }] }),
		),
	)

	renderScreen()

	expect(await screen.findByRole('alert')).toBeInTheDocument()
})

test('the add form asks for no name', async () => {
	serveFieldCatalogue([])

	renderScreen()
	await screen.findByText(/No fields yet/i)

	expect(screen.getByLabelText('Label')).toBeInTheDocument()
	expect(screen.queryByLabelText('Name')).not.toBeInTheDocument()
})

test('defining a field sends the name its label makes', async () => {
	serveFieldCatalogue([])
	const defined = captureDefine()

	renderScreen()
	await defineLabelled('Birth date')

	await waitFor(() =>
		expect(defined).toHaveBeenCalledWith({
			name: 'birthDate',
			label: 'Birth date',
			kind: 'TEXT',
		}),
	)
})

test('numbers a name a live field holds', async () => {
	serveFieldCatalogue([{ ...birthDate, label: 'Date of birth' }])
	const defined = captureDefine()

	renderScreen()
	await defineLabelled('Birth date')

	expect(await sentName(defined)).toBe('birthDate2')
})

test('numbers a name an archived field holds', async () => {
	serveFieldCatalogue([], [birthDate])
	const defined = captureDefine()

	renderScreen()
	await defineLabelled('Birth date')

	expect(await sentName(defined)).toBe('birthDate2')
})

test('refuses a label a live field already has', async () => {
	serveFieldCatalogue([birthDate])
	const defined = captureDefine()

	renderScreen()
	await defineLabelled(' birth DATE ')

	expect(await screen.findByRole('alert')).toHaveTextContent('A field with that label already exists.')
	expect(defined).not.toHaveBeenCalled()
})

test('a refused label replaces the notice of an earlier failure', async () => {
	serveFieldCatalogue([birthDate])
	server.use(graphql.mutation('DefineField', () => HttpResponse.json(refusal('VALIDATION', 'field_label_too_long'))))

	renderScreen()
	await defineLabelled('Anniversary')
	await screen.findByRole('alert')
	await userEvent.clear(screen.getByLabelText('Label'))
	await defineLabelled('Birth date')

	expect(await screen.findByText('A field with that label already exists.')).toBeInTheDocument()
	expect(screen.getAllByRole('alert')).toHaveLength(1)
})

test('typing another label clears the label notice', async () => {
	serveFieldCatalogue([birthDate])

	renderScreen()
	await defineLabelled('Birth date')
	await screen.findByText('A field with that label already exists.')
	await userEvent.type(screen.getByLabelText('Label'), 's')

	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('steps past a name the contact already has', async () => {
	serveFieldCatalogue([])
	const defined = captureDefine()

	renderScreen()
	await defineLabelled('Name')

	expect(await sentName(defined)).toBe('name2')
})

test('steps past a name every object has', async () => {
	serveFieldCatalogue([])
	const defined = captureDefine()

	renderScreen()
	await defineLabelled('Constructor')

	expect(await sentName(defined)).toBe('constructor2')
})

test('steps past the names the server lists', async () => {
	serveFieldCatalogue([], [], ['birthDate'])
	const defined = captureDefine()

	renderScreen()
	await defineLabelled('Birth date')

	expect(await sentName(defined)).toBe('birthDate2')
})

test('names a label with no letters or digits field2', async () => {
	serveFieldCatalogue([])
	const defined = captureDefine()

	renderScreen()
	await defineLabelled('???')

	expect(await sentName(defined)).toBe('field2')
})

test('numbers a label ending in a digit straight after it', async () => {
	serveFieldCatalogue([{ ...birthDate, name: 'address2', label: 'Second address', kind: 'TEXT' }])
	const defined = captureDefine()

	renderScreen()
	await defineLabelled('Address 2')

	expect(await sentName(defined)).toBe('address22')
})

test('a second visit reads the catalogue from the server again', async () => {
	let reads = 0
	server.use(
		graphql.query('FieldCatalogue', () => {
			reads += 1
			return HttpResponse.json({ data: { fields: [], every: [], reservedFieldNames: RESERVED } })
		}),
	)
	const { graph } = fakeGraphClient()

	const first = renderScreen(graph)
	await screen.findByText(/No fields yet/i)
	first.unmount()
	renderScreen(graph)

	await waitFor(() => expect(reads).toBe(2))
})

async function submitField() {
	await screen.findByText(/No fields yet/i)
	await defineLabelled('Birth date')
}

test('a taken name is reported word for word', async () => {
	serveFieldCatalogue([])
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

test('a name taken meanwhile asks to press Add field again', async () => {
	speakTemplates()
	serveFieldCatalogue([])
	server.use(graphql.mutation('DefineField', () => HttpResponse.json(refusal('CONFLICT', 'field_name_taken'))))

	renderScreen()
	await submitField()

	expect(await screen.findByRole('alert')).toHaveTextContent('The field list just changed. Press Add field again.')
})

test('an archived field holding the name asks to press Add field again', async () => {
	speakTemplates()
	serveFieldCatalogue([])
	server.use(graphql.mutation('DefineField', () => HttpResponse.json(refusal('CONFLICT', 'field_kind_locked'))))

	renderScreen()
	await submitField()

	expect(await screen.findByRole('alert')).toHaveTextContent('The field list just changed. Press Add field again.')
})

test('the catalogue is read again after a refused define', async () => {
	let reads = 0
	server.use(
		graphql.query('FieldCatalogue', () => {
			reads += 1
			return HttpResponse.json({ data: { fields: [], every: [], reservedFieldNames: RESERVED } })
		}),
		graphql.mutation('DefineField', () => HttpResponse.json(refusal('VALIDATION', 'field_label_too_long'))),
	)

	renderScreen()
	await submitField()
	await screen.findByRole('alert')

	await waitFor(() => expect(reads).toBe(2))
})

test('a refused name is numbered on the next press', async () => {
	serveFieldCatalogue([])
	const defined = vi.fn()
	server.use(
		graphql.mutation('DefineField', async ({ variables }) => {
			defined(variables)
			return HttpResponse.json(refusal('CONFLICT', 'field_name_taken'))
		}),
	)

	renderScreen()
	await submitField()
	await waitFor(() => expect(defined).toHaveBeenCalledTimes(1))
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

	await waitFor(() => expect(defined).toHaveBeenCalledTimes(2))
	expect(defined.mock.calls.map((call) => call[0].name)).toEqual(['birthDate', 'birthDate2'])
})

test('the chosen kind is sent with the definition', async () => {
	serveFieldCatalogue([])
	const defined = captureDefine()

	renderScreen()
	await screen.findByText(/No fields yet/i)
	await userEvent.type(await screen.findByLabelText('Label'), 'Birth date')
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
	serveFieldCatalogue([])
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
		graphql.query('FieldCatalogue', () =>
			HttpResponse.json({ data: { fields: served, every: served, reservedFieldNames: RESERVED } }),
		),
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
	server.use(graphql.query('FieldCatalogue', () => HttpResponse.json({ data: {} })))
	const defined = captureDefine()

	renderScreen()
	expect(await screen.findByText(/No fields yet/i)).toBeInTheDocument()
	await defineLabelled('Birth date')

	expect(await sentName(defined)).toBe('birthDate')
})

test('a failed archive is reported', async () => {
	serveFieldCatalogue([birthDate])
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
	serveFieldCatalogue([])
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

const history = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000506',
	name: 'history',
	label: 'History',
	kind: 'REPEATER',
	subFields: [
		{ __typename: 'FieldSubField', name: 'date', label: 'Date', kind: 'DATE' },
		{ __typename: 'FieldSubField', name: 'comment', label: 'Comment', kind: 'LONGTEXT' },
	],
}

async function startRepeater() {
	await screen.findByText(/No fields yet/i)
	await userEvent.type(await screen.findByLabelText('Label'), 'History')
	await userEvent.click(screen.getByRole('combobox', { name: 'Kind' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Repeater' }))
}

async function addSubField(label: string, kind: string) {
	await userEvent.click(screen.getByRole('button', { name: 'Add sub field' }))
	const rows = screen.getAllByRole('group', { name: /^Sub field \d+$/ })
	const row = within(rows[rows.length - 1])
	await userEvent.type(row.getByLabelText('Label'), label)
	await userEvent.click(row.getByRole('combobox', { name: 'Kind' }))
	await userEvent.click(await screen.findByRole('option', { name: kind }))
}

test('the catalogue shows a repeater beside the labels of its sub fields', async () => {
	serveFieldCatalogue([history])

	renderScreen()

	const table = await screen.findByRole('table')
	const cells = within(within(table).getAllByRole('row')[1]).getAllByRole('cell')
	expect(cells[2]).toHaveTextContent('Repeater')
	expect(cells[2]).toHaveTextContent('Date, Comment')
})

test('a field holding no sub fields shows its kind alone', async () => {
	serveFieldCatalogue([birthDate])

	renderScreen()

	const table = await screen.findByRole('table')
	const cells = within(within(table).getAllByRole('row')[1]).getAllByRole('cell')
	expect(cells[2]).toHaveTextContent(/^Date$/)
})

test('only the repeater kind asks for sub fields', async () => {
	serveFieldCatalogue([])

	renderScreen()
	await screen.findByText(/No fields yet/i)

	expect(screen.queryByRole('button', { name: 'Add sub field' })).not.toBeInTheDocument()
	await userEvent.click(screen.getByRole('combobox', { name: 'Kind' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Repeater' }))
	expect(screen.getByText('No sub fields yet.')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Add sub field' })).toBeInTheDocument()
})

test('defining a repeater sends sub fields named from their labels', async () => {
	serveFieldCatalogue([])
	const defined = captureDefine(history)

	renderScreen()
	await startRepeater()
	await addSubField('Date', 'Date')
	await addSubField('Follow-up comment', 'Long text')
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

	await waitFor(() =>
		expect(defined).toHaveBeenCalledWith({
			name: 'history',
			label: 'History',
			kind: 'REPEATER',
			subFields: [
				{ name: 'date', label: 'Date', kind: 'DATE' },
				{ name: 'followUpComment', label: 'Follow-up comment', kind: 'LONGTEXT' },
			],
		}),
	)
	expect(defined).toHaveBeenCalledTimes(1)
})

test('two sub fields sharing a label are sent under distinct names', async () => {
	serveFieldCatalogue([])
	const defined = captureDefine(history)

	renderScreen()
	await startRepeater()
	await addSubField('Note', 'Text')
	await addSubField('Note', 'Text')
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

	await waitFor(() => expect(defined).toHaveBeenCalledTimes(1))
	expect(defined.mock.calls[0][0].subFields.map((column: { name: string }) => column.name)).toEqual([
		'note',
		'note2',
	])
})

test('a sub field labelled ID is sent under a name other than the one entries keep their id under', async () => {
	serveFieldCatalogue([])
	const defined = captureDefine(history)

	renderScreen()
	await startRepeater()
	await addSubField('ID', 'Text')
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

	await waitFor(() => expect(defined).toHaveBeenCalledTimes(1))
	expect(defined.mock.calls[0][0].subFields.map((column: { name: string }) => column.name)).toEqual(['id2'])
})

test('the sub field kind menu leaves the repeater out', async () => {
	serveFieldCatalogue([])

	renderScreen()
	await startRepeater()
	await userEvent.click(screen.getByRole('button', { name: 'Add sub field' }))
	const row = within(screen.getByRole('group', { name: 'Sub field 1' }))
	await userEvent.click(row.getByRole('combobox', { name: 'Kind' }))

	expect(await screen.findByRole('option', { name: 'Long text' })).toBeInTheDocument()
	expect(screen.queryByRole('option', { name: 'Repeater' })).not.toBeInTheDocument()
})

test('a field moved off the repeater kind sends no sub fields', async () => {
	serveFieldCatalogue([])
	const defined = captureDefine(history)

	renderScreen()
	await startRepeater()
	await addSubField('Date', 'Date')
	await userEvent.click(screen.getAllByRole('combobox', { name: 'Kind' })[0])
	await userEvent.click(await screen.findByRole('option', { name: 'Text' }))
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

	await waitFor(() => expect(defined).toHaveBeenCalledTimes(1))
	expect(defined.mock.calls[0][0]).toEqual({ name: 'history', label: 'History', kind: 'TEXT' })
	expect(defined.mock.calls[0][0]).not.toHaveProperty('subFields')
})

test('a defined repeater clears its sub fields for the next one', async () => {
	serveFieldCatalogue([])
	captureDefine(history)

	renderScreen()
	await startRepeater()
	await addSubField('Date', 'Date')
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

	expect(await screen.findByText('No sub fields yet.')).toBeInTheDocument()
	expect(screen.queryByRole('group', { name: 'Sub field 1' })).not.toBeInTheDocument()
})

test('archiving a field sends its id', async () => {
	serveFieldCatalogue([birthDate])
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

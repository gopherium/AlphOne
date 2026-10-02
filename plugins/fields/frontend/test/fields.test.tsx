// SPDX-License-Identifier: AGPL-3.0-or-later

import { configureErrorText, rememberFormatLocale } from '@alphone/frontend-sdk'
import {
	HttpResponse,
	fakeGraphClient,
	graphql,
	server,
	textClasses,
} from '@alphone/frontend-sdk/testing'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { FieldsScreen } from '../FieldsScreen'
import { capture, hold, refusal, renderHosted, speakTemplates } from './harness'

/** scrolled records every scroll into view an element asks for. */
const scrolled = vi.fn()

beforeEach(() => {
	scrolled.mockClear()
	Element.prototype.scrollIntoView = scrolled
})

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

/**
 * Renders the Fields screen inside its graph and query providers, below a toaster.
 * @param graph - The graph client the screen reads through.
 * @returns The render result.
 */
function renderScreen(graph = fakeGraphClient().graph) {
	return renderHosted(<FieldsScreen />, graph)
}

/**
 * Answers the Fields screen catalogue with the given fields and reserved names.
 * @param live - The live fields.
 * @param archived - The archived fields.
 * @param reserved - The names the server refuses.
 */
function serveFieldCatalogue(live: unknown[], archived: unknown[] = [], reserved: string[] = RESERVED) {
	server.use(
		graphql.query('FieldCatalogue', () =>
			HttpResponse.json({ data: { fields: live, every: [...live, ...archived], reservedFieldNames: reserved } }),
		),
	)
}

/**
 * Answers every define with the given field and records the variables it was sent.
 * @param answer - The field the define answers.
 * @returns The recorder of the variables.
 */
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

/**
 * Types a label into the add form and presses Add field.
 * @param label - The label to type.
 */
async function defineLabelled(label: string) {
	await userEvent.type(await screen.findByLabelText('Label'), label)
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))
}

/**
 * Returns the name the only define sent.
 * @param defined - The recorder of the define variables.
 * @returns The name, once the define was sent.
 */
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
	expect(cells[1]).toHaveTextContent(/^Date$/)
	expect(cells[2]).toHaveTextContent('birthDate')
	const headers = within(table).getAllByRole('columnheader')
	expect(headers.map((header) => header.textContent)).toEqual(['Label', 'Kind', 'API name', ''])
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
	expect(await screen.findByText('Field added.')).toBeInTheDocument()
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

/** Defines Birth date from the add form of an empty catalogue. */
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
	expect(screen.queryByText('Field added.')).not.toBeInTheDocument()
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

test.each(['field_name_taken', 'field_kind_locked'])(
	'a name refused with %s is numbered on the next press',
	async (reason) => {
		serveFieldCatalogue([])
		const defined = vi.fn()
		server.use(
			graphql.mutation('DefineField', async ({ variables }) => {
				defined(variables)
				return HttpResponse.json(refusal('CONFLICT', reason))
			}),
		)

		renderScreen()
		await submitField()
		await screen.findByRole('alert')
		await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

		await waitFor(() => expect(defined).toHaveBeenCalledTimes(2))
		expect(defined.mock.calls.map((call) => call[0].name)).toEqual(['birthDate', 'birthDate2'])
	},
)

test('a refusal that is not a race sends the same name again', async () => {
	serveFieldCatalogue([])
	const defined = vi.fn()
	server.use(
		graphql.mutation('DefineField', async ({ variables }) => {
			defined(variables)
			return HttpResponse.json(
				defined.mock.calls.length === 1
					? refusal('VALIDATION', 'field_label_too_long')
					: { data: { defineField: birthDate } },
			)
		}),
	)

	renderScreen()
	await submitField()
	await screen.findByRole('alert')
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))

	await waitFor(() => expect(defined).toHaveBeenCalledTimes(2))
	expect(defined.mock.calls.map((call) => call[0].name)).toEqual(['birthDate', 'birthDate'])
})

test('a define lost on the way keeps the form and its draft', async () => {
	serveFieldCatalogue([])
	server.use(graphql.mutation('DefineField', () => HttpResponse.error()))
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await defineLabelled('Anniversary')

	expect(await screen.findByRole('alert')).toHaveTextContent('The field could not be defined.')
	expect(graph.refetch).not.toHaveBeenCalled()
	expect(screen.getByLabelText('Label')).toHaveValue('Anniversary')
	expect(screen.queryByText('Field added.')).not.toBeInTheDocument()
})

test('a refused label drops an earlier failure for good', async () => {
	serveFieldCatalogue([birthDate])
	server.use(graphql.mutation('DefineField', () => HttpResponse.json(refusal('VALIDATION', 'field_label_too_long'))))

	renderScreen()
	await defineLabelled('Anniversary')
	await screen.findByRole('alert')
	await userEvent.clear(screen.getByLabelText('Label'))
	await defineLabelled('Birth date')
	await screen.findByText('A field with that label already exists.')
	await userEvent.type(screen.getByLabelText('Label'), 's')

	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
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

/** notFound is the answer of an archive naming no live field. */
const notFound = {
	errors: [{ message: 'fields: no live definition holds that id', extensions: { code: 'NOT_FOUND' } }],
}

/**
 * Presses the trash of the named field, then Archive in the question it asks.
 * @param label - The label of the field.
 */
async function archiveField(label: string) {
	await userEvent.click(await screen.findByRole('button', { name: `Archive ${label}` }))
	await userEvent.click(screen.getByRole('button', { name: 'Archive' }))
}

/**
 * Returns the table row showing the named field.
 * @param label - The label of the field.
 * @returns The row.
 */
function rowOf(label: string) {
	return screen.getByText(label).closest('tr') as HTMLElement
}

test('a failed archive is reported', async () => {
	serveFieldCatalogue([birthDate])
	server.use(graphql.mutation('ArchiveField', () => HttpResponse.json(notFound)))

	renderScreen()
	await archiveField('Birth date')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The field could not be archived.',
	)
	expect(screen.queryByText('Field archived.')).not.toBeInTheDocument()
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

/** Starts a repeater labelled History in the add form of an empty catalogue. */
async function startRepeater() {
	await screen.findByText(/No fields yet/i)
	await userEvent.type(await screen.findByLabelText('Label'), 'History')
	await userEvent.click(screen.getByRole('combobox', { name: 'Kind' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Repeater' }))
}

/**
 * Adds one sub field to the repeater the add form holds.
 * @param label - The sub field label.
 * @param kind - The kind option to choose.
 */
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
	expect(cells[1]).toHaveTextContent('Repeater')
	expect(cells[1]).toHaveTextContent('Date, Comment')
})

test('a field holding no sub fields shows its kind alone', async () => {
	serveFieldCatalogue([birthDate])

	renderScreen()

	const table = await screen.findByRole('table')
	const cells = within(within(table).getAllByRole('row')[1]).getAllByRole('cell')
	expect(cells[1]).toHaveTextContent(/^Date$/)
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

test('a sub field is numbered in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	serveFieldCatalogue([])

	renderScreen()
	await startRepeater()
	await userEvent.click(screen.getByRole('button', { name: 'Add sub field' }))

	expect(screen.getByRole('group', { name: 'Sub field १' })).toBeInTheDocument()
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

test('lays the label and the kind on one form row', async () => {
	serveFieldCatalogue([])
	server.use(graphql.mutation('DefineField', () => HttpResponse.error()))

	renderScreen()
	await startRepeater()
	await userEvent.click(screen.getByRole('button', { name: 'Add sub field' }))
	await userEvent.click(screen.getByRole('button', { name: 'Add field' }))
	const notice = await screen.findByRole('alert')

	const label = screen.getAllByLabelText('Label')[0]
	const kind = screen.getAllByRole('combobox', { name: 'Kind' })[0]
	const row = label.closest('.godmin-form__row') as HTMLElement
	expect(row).not.toBeNull()
	expect(row.parentElement).toHaveClass('godmin-form')
	expect(row.children).toHaveLength(2)
	expect(row.children[0]).toContainElement(label)
	expect(row.children[1]).toContainElement(kind)
	expect(notice).toHaveTextContent('The field could not be defined.')
	expect(notice.closest('.godmin-form__row')).toBeNull()
	expect(screen.getByRole('group', { name: 'Sub field 1' }).closest('.godmin-form__row')).toBeNull()
	expect(screen.getByRole('button', { name: 'Add field' }).closest('.godmin-form__row')).toBeNull()
})

test('lays a sub field label and kind on one row beside a trash icon that removes it', async () => {
	serveFieldCatalogue([])

	renderScreen()
	await startRepeater()
	await userEvent.click(screen.getByRole('button', { name: 'Add sub field' }))

	const group = screen.getByRole('group', { name: 'Sub field 1' })
	const label = within(group).getByLabelText('Label')
	const kind = within(group).getByRole('combobox', { name: 'Kind' })
	const row = label.closest('.godmin-form__row') as HTMLElement
	expect(row).not.toBeNull()
	expect(group).toContainElement(row)
	expect(row.children).toHaveLength(2)
	expect(row.children[0]).toContainElement(label)
	expect(row.children[1]).toContainElement(kind)
	const remove = within(group).getByRole('button', { name: 'Remove sub field' })
	expect(remove.textContent).toBe('')
	expect(remove.querySelector('svg')).not.toBeNull()
	expect(remove.closest('.godmin-rows__line')).toContainElement(row)

	await userEvent.click(remove)

	expect(screen.queryByRole('group', { name: 'Sub field 1' })).not.toBeInTheDocument()
	expect(screen.getByText('No sub fields yet.')).toBeInTheDocument()
})

test('fills the page with the add form and gives every label the extra room over its kind', async () => {
	serveFieldCatalogue([])

	renderScreen()
	await startRepeater()
	await userEvent.click(screen.getByRole('button', { name: 'Add sub field' }))
	await userEvent.click(screen.getByRole('button', { name: 'Add sub field' }))

	const label = screen.getAllByLabelText('Label')[0]
	const row = label.closest('.godmin-form__row') as HTMLElement
	expect(row.closest('form')).toHaveClass('godmin-form', 'godmin-form--inline')
	const grown = row.querySelectorAll('.godmin-form__grow')
	expect(grown).toHaveLength(1)
	expect(grown[0].parentElement).toBe(row)
	expect(grown[0]).toContainElement(label)
	const groups = screen.getAllByRole('group', { name: /^Sub field \d+$/ })
	expect(groups).toHaveLength(2)
	for (const group of groups) {
		const subLabel = within(group).getByLabelText('Label')
		const kind = within(group).getByRole('combobox', { name: 'Kind' })
		const subRow = subLabel.closest('.godmin-form__row') as HTMLElement
		const subGrown = subRow.querySelectorAll('.godmin-form__grow')
		expect(subGrown).toHaveLength(1)
		expect(subGrown[0].parentElement).toBe(subRow)
		expect(subGrown[0]).toContainElement(subLabel)
		expect(subRow.children[1]).toContainElement(kind)
		expect(subRow.children[1]).not.toHaveClass('godmin-form__grow')
	}
})

test('sets the Add a field and Sub fields headings a size above the field labels', async () => {
	serveFieldCatalogue([])

	renderScreen()
	await startRepeater()

	const add = screen.getByRole('heading', { level: 2, name: 'Add a field' })
	const sub = screen.getByRole('heading', { level: 3, name: 'Sub fields' })
	expect([...add.classList]).toEqual(expect.arrayContaining(textClasses('heading-lg')))
	expect([...sub.classList]).toEqual(expect.arrayContaining(textClasses('heading-md')))
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
	await archiveField('Birth date')

	await waitFor(() => expect(archived).toHaveBeenCalledWith({ id: birthDate.id }))
	expect(await screen.findByText('Field archived.')).toBeInTheDocument()
})

const shoeSize = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000502',
	name: 'shoeSize',
	label: 'Shoe size',
	kind: 'NUMBER',
	subFields: [],
}

const nickname = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000503',
	name: 'nickname',
	label: 'Nickname',
	kind: 'TEXT',
	subFields: [],
}

const hatSize = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000504',
	name: 'hatSize',
	label: 'Hat size',
	kind: 'TEXT',
	subFields: [],
}

const faxNumber = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000505',
	name: 'faxNumber',
	label: 'Fax number',
	kind: 'TEXT',
	subFields: [],
}

/** accepted is the answer of an order the server saved. */
const accepted = { data: { orderFields: true } }

/**
 * Returns the labels of the catalogue rows in the order the table shows them.
 * @returns The labels, top row first.
 */
async function shownLabels() {
	const table = await screen.findByRole('table')
	return within(table)
		.getAllByRole('row')
		.slice(1)
		.map((row) => within(row).getAllByRole('cell')[0].textContent)
}

/**
 * Serves the catalogue from a store that orders, archives and defines fields, holding every order until released.
 * @param live - The live fields in their stored order.
 * @returns The recorder of the orders, their release, the count of catalogue reads and a setter of the live fields.
 */
function serveFieldStore(live: (typeof birthDate)[]) {
	let served = live
	let reads = 0
	const ordered = vi.fn()
	let release = () => {}
	const released = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		graphql.query('FieldCatalogue', () => {
			reads += 1
			return HttpResponse.json({ data: { fields: served, every: served, reservedFieldNames: RESERVED } })
		}),
		graphql.mutation('OrderFields', async ({ variables }) => {
			ordered(variables)
			await released
			served = served
				.filter((field) => variables.ids.includes(field.id))
				.sort((one, other) => variables.ids.indexOf(one.id) - variables.ids.indexOf(other.id))
			return HttpResponse.json(accepted)
		}),
		graphql.mutation('ArchiveField', ({ variables }) => {
			served = served.filter((field) => field.id !== variables.id)
			return HttpResponse.json({ data: { archiveField: true } })
		}),
		graphql.mutation('DefineField', () => {
			served = [...served, hatSize]
			return HttpResponse.json({ data: { defineField: hatSize } })
		}),
	)
	return {
		ordered,
		release: () => release(),
		reads: () => reads,
		serve: (fields: (typeof birthDate)[]) => {
			served = fields
		},
	}
}

/** Lets every request the screen started reach the server. */
async function settle() {
	await act(() => new Promise((resolve) => setTimeout(resolve, 50)))
}

test('each field row carries arrows that move it and a trash icon that asks to archive it', async () => {
	serveFieldCatalogue([birthDate])

	renderScreen()

	const archive = await screen.findByRole('button', { name: 'Archive Birth date' })
	expect(archive.textContent).toBe('')
	expect(archive.querySelector('svg')).not.toBeNull()
	expect(screen.getByRole('button', { name: 'Move Birth date up' })).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Move Birth date down' })).toBeInTheDocument()
	expect(screen.queryByText('Archive')).not.toBeInTheDocument()
})

test('the trash asks first and puts focus on Keep', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	const archived = capture('ArchiveField', { data: { archiveField: true } })

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Archive Birth date' }))

	const row = within(rowOf('Birth date'))
	expect(row.getByRole('group', { name: 'Archive this field?' })).toBeInTheDocument()
	expect(row.getByRole('button', { name: 'Archive' })).toBeInTheDocument()
	expect(row.getByRole('button', { name: 'Keep' })).toHaveFocus()
	expect(row.queryByRole('button', { name: 'Archive Birth date' })).not.toBeInTheDocument()
	expect(row.queryByRole('button', { name: 'Move Birth date down' })).not.toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Archive Shoe size' })).not.toHaveAttribute('aria-disabled', 'true')
	const group = row.getByRole('group', { name: 'Archive this field?' })
	const [question, buttons] = [...group.children] as HTMLElement[]
	expect(group.style.flexDirection).toBe('column')
	expect(question).toHaveTextContent(/^Archive this field\?$/)
	expect(buttons.style.flexDirection).toBe('row')
	expect(buttons.style.flexWrap).toBe('nowrap')
	expect(buttons).toContainElement(row.getByRole('button', { name: 'Archive' }))
	expect(buttons).toContainElement(row.getByRole('button', { name: 'Keep' }))
	await settle()
	expect(archived).not.toHaveBeenCalled()
})

test('a trash pressed while another row asks moves the question to its row and puts focus on its Keep', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	const archived = capture('ArchiveField', { data: { archiveField: true } })

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Archive Birth date' }))
	await userEvent.click(screen.getByRole('button', { name: 'Archive Shoe size' }))

	expect(screen.getAllByRole('group', { name: 'Archive this field?' })).toHaveLength(1)
	const shoe = within(rowOf('Shoe size'))
	expect(shoe.getByRole('group', { name: 'Archive this field?' })).toBeInTheDocument()
	expect(shoe.getByRole('button', { name: 'Keep' })).toHaveFocus()
	expect(within(rowOf('Birth date')).getByRole('button', { name: 'Archive Birth date' })).toBeInTheDocument()
	await settle()
	expect(archived).not.toHaveBeenCalled()
})

test('Keep leaves the field and puts focus back on its trash', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	const archived = capture('ArchiveField', { data: { archiveField: true } })

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Archive Birth date' }))
	await userEvent.click(screen.getByRole('button', { name: 'Keep' }))

	expect(screen.queryByRole('group', { name: 'Archive this field?' })).not.toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Archive Birth date' })).toHaveFocus()
	expect(screen.getByRole('button', { name: 'Move Birth date down' })).toBeInTheDocument()
	await settle()
	expect(archived).not.toHaveBeenCalled()
})

test.each([
	['lost on the way', () => HttpResponse.error()],
	['refused', () => HttpResponse.json(notFound)],
])('an archive %s closes the question and puts focus back on the trash', async (_, answer) => {
	serveFieldCatalogue([birthDate, shoeSize])
	server.use(graphql.mutation('ArchiveField', answer))

	renderScreen()
	await archiveField('Birth date')

	expect(await screen.findByRole('alert')).toHaveTextContent('The field could not be archived.')
	expect(screen.queryByRole('group', { name: 'Archive this field?' })).not.toBeInTheDocument()
	const trash = screen.getByRole('button', { name: 'Archive Birth date' })
	await waitFor(() => expect(trash).toHaveFocus())
	expect(trash).not.toHaveAttribute('aria-disabled', 'true')
	expect(screen.getByRole('button', { name: 'Archive Shoe size' })).not.toHaveAttribute('aria-disabled', 'true')
})

test('a failed archive leaves focus where the reader moved it', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	const archiving = hold('ArchiveField', notFound)

	renderScreen()
	await archiveField('Birth date')
	await waitFor(() => expect(archiving.called).toHaveBeenCalledOnce())
	const label = screen.getByLabelText('Label')
	await userEvent.click(label)
	archiving.release()

	expect(await screen.findByRole('alert')).toHaveTextContent('The field could not be archived.')
	await settle()
	expect(label).toHaveFocus()
})

test('the arrows at the ends of the list are marked disabled and send nothing', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	const ordered = capture('OrderFields', accepted)

	renderScreen()
	const top = await screen.findByRole('button', { name: 'Move Birth date up' })
	const bottom = screen.getByRole('button', { name: 'Move Shoe size down' })
	expect(top).toHaveAttribute('aria-disabled', 'true')
	expect(bottom).toHaveAttribute('aria-disabled', 'true')
	expect(screen.getByRole('button', { name: 'Move Birth date down' })).not.toHaveAttribute('aria-disabled', 'true')
	expect(screen.getByRole('button', { name: 'Move Shoe size up' })).not.toHaveAttribute('aria-disabled', 'true')
	await userEvent.click(top)
	await userEvent.click(bottom)
	await userEvent.click(screen.getByRole('button', { name: 'Move Shoe size up' }))

	await waitFor(() => expect(ordered).toHaveBeenCalledTimes(1))
	expect(ordered).toHaveBeenCalledWith({ ids: [shoeSize.id, birthDate.id] })
})

test('a move shows the new order at once and sends only the live fields in it', async () => {
	serveFieldCatalogue([birthDate, shoeSize, nickname], [faxNumber])
	const ordered = hold('OrderFields', accepted)

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Move Nickname up' }))

	expect(await shownLabels()).toEqual(['Birth date', 'Nickname', 'Shoe size'])
	await waitFor(() => expect(ordered.called).toHaveBeenCalledTimes(1))
	expect(ordered.called).toHaveBeenCalledWith({ ids: [birthDate.id, nickname.id, shoeSize.id] })
	ordered.release()
})

test('a move while an order is unanswered waits for its answer, sends the latest order, then reads once', async () => {
	const store = serveFieldStore([birthDate, shoeSize, nickname])
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))
	await userEvent.click(screen.getByRole('button', { name: 'Move Birth date down' }))
	expect(await shownLabels()).toEqual(['Shoe size', 'Nickname', 'Birth date'])
	await waitFor(() => expect(store.ordered).toHaveBeenCalledTimes(1))
	await settle()

	expect(store.ordered).toHaveBeenCalledTimes(1)
	expect(store.ordered).toHaveBeenCalledWith({ ids: [shoeSize.id, birthDate.id, nickname.id] })
	store.release()
	await waitFor(() => expect(store.ordered).toHaveBeenCalledTimes(2))
	expect(store.ordered).toHaveBeenLastCalledWith({ ids: [shoeSize.id, nickname.id, birthDate.id] })
	await waitFor(() => expect(store.reads()).toBe(2))
	await settle()
	expect(store.reads()).toBe(2)
	expect(graph.refetch).toHaveBeenCalledTimes(1)
	expect(await shownLabels()).toEqual(['Shoe size', 'Nickname', 'Birth date'])
})

test('a move pressed as an order answer lands sends the order it shows', async () => {
	const store = serveFieldStore([birthDate, shoeSize, nickname])
	const { graph } = fakeGraphClient()
	const refetch = vi.mocked(graph.refetch)
	const read = refetch.getMockImplementation() as typeof graph.refetch
	refetch.mockImplementationOnce((operations) => {
		read(operations)
		screen.getByRole('button', { name: 'Move Birth date down' }).click()
	})

	renderScreen(graph)
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))
	await waitFor(() => expect(store.ordered).toHaveBeenCalledTimes(1))
	store.release()

	await waitFor(() => expect(store.ordered).toHaveBeenCalledTimes(2))
	expect(store.ordered).toHaveBeenLastCalledWith({ ids: [shoeSize.id, nickname.id, birthDate.id] })
	await waitFor(() => expect(store.reads()).toBe(3))
	await settle()
	expect(await shownLabels()).toEqual(['Shoe size', 'Nickname', 'Birth date'])
})

test('a refused order drops the moves made while it was unanswered and shows the fields as they are', async () => {
	speakTemplates()
	let served = [birthDate, shoeSize, nickname]
	server.use(
		graphql.query('FieldCatalogue', () =>
			HttpResponse.json({ data: { fields: served, every: served, reservedFieldNames: RESERVED } }),
		),
	)
	const refused = hold('OrderFields', refusal('CONFLICT', 'field_order_incomplete'))
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))
	await userEvent.click(screen.getByRole('button', { name: 'Move Birth date down' }))
	expect(await shownLabels()).toEqual(['Shoe size', 'Nickname', 'Birth date'])
	served = [birthDate, shoeSize, nickname, hatSize]
	refused.release()

	expect(await screen.findByRole('alert')).toHaveTextContent('The field list just changed. The new order was not saved.')
	expect(await screen.findByRole('button', { name: 'Archive Hat size' })).toBeInTheDocument()
	expect(await shownLabels()).toEqual(['Birth date', 'Shoe size', 'Nickname', 'Hat size'])
	await settle()
	expect(refused.called).toHaveBeenCalledTimes(1)
	expect(graph.refetch).toHaveBeenCalledTimes(1)
})

test('an order lost after an earlier one was saved keeps the order the earlier one saved', async () => {
	serveFieldCatalogue([birthDate, shoeSize, nickname])
	const ordered = vi.fn()
	let answerFirst = () => {}
	const first = new Promise<void>((resolve) => {
		answerFirst = resolve
	})
	server.use(
		graphql.mutation('OrderFields', async ({ variables }) => {
			ordered(variables)
			if (ordered.mock.calls.length > 1) {
				return HttpResponse.error()
			}
			await first
			return HttpResponse.json(accepted)
		}),
	)
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))
	await userEvent.click(screen.getByRole('button', { name: 'Move Birth date down' }))
	await waitFor(() => expect(ordered).toHaveBeenCalledTimes(1))
	answerFirst()

	expect(await screen.findByRole('alert')).toHaveTextContent('The fields could not be ordered.')
	expect(await shownLabels()).toEqual(['Shoe size', 'Birth date', 'Nickname'])
	expect(ordered).toHaveBeenCalledTimes(2)
	await userEvent.click(screen.getByRole('button', { name: 'Move Nickname up' }))
	await waitFor(() => expect(ordered).toHaveBeenCalledTimes(3))
	await settle()
	expect(await shownLabels()).toEqual(['Shoe size', 'Birth date', 'Nickname'])
	expect(graph.refetch).not.toHaveBeenCalled()
})

test('a field archived while an order is unanswered leaves the list and the next order', async () => {
	const store = serveFieldStore([birthDate, shoeSize, nickname])

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Move Nickname up' }))
	await archiveField('Shoe size')
	await waitFor(() => expect(screen.queryByText('Shoe size')).not.toBeInTheDocument())

	expect(await shownLabels()).toEqual(['Birth date', 'Nickname'])
	await userEvent.click(screen.getByRole('button', { name: 'Move Nickname up' }))
	store.release()
	await waitFor(() => expect(store.ordered).toHaveBeenCalledTimes(2))
	expect(store.ordered).toHaveBeenLastCalledWith({ ids: [nickname.id, birthDate.id] })
})

test('a field defined while an order is unanswered joins the end of the order on screen', async () => {
	const store = serveFieldStore([birthDate, shoeSize, nickname])

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))
	await defineLabelled('Hat size')

	expect(await screen.findByRole('button', { name: 'Archive Hat size' })).toBeInTheDocument()
	expect(await shownLabels()).toEqual(['Shoe size', 'Birth date', 'Nickname', 'Hat size'])
	store.release()
	await waitFor(() => expect(store.ordered).toHaveBeenCalledTimes(2))
	expect(store.ordered).toHaveBeenLastCalledWith({ ids: [shoeSize.id, birthDate.id, nickname.id, hatSize.id] })
})

test('a saved order reads the catalogue again and shows what the server answers', async () => {
	let served = [birthDate, shoeSize]
	server.use(
		graphql.query('FieldCatalogue', () =>
			HttpResponse.json({ data: { fields: served, every: served, reservedFieldNames: RESERVED } }),
		),
		graphql.mutation('OrderFields', () => {
			served = [nickname, shoeSize, birthDate]
			return HttpResponse.json(accepted)
		}),
	)
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))

	await waitFor(() => expect(graph.refetch).toHaveBeenCalledWith(['FieldCatalogue']))
	await waitFor(async () => expect(await shownLabels()).toEqual(['Nickname', 'Shoe size', 'Birth date']))
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('a failed order puts the rows back and says so', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	server.use(graphql.mutation('OrderFields', () => HttpResponse.json({ errors: [{ message: 'boom' }] })))

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('The fields could not be ordered.')
	expect(await shownLabels()).toEqual(['Birth date', 'Shoe size'])
})

test('a refused order says it was not saved and shows the server order at once', async () => {
	speakTemplates()
	const reads = vi.fn()
	let releaseRead = () => {}
	const reread = new Promise<void>((resolve) => {
		releaseRead = resolve
	})
	server.use(
		graphql.query('FieldCatalogue', async () => {
			reads()
			if (reads.mock.calls.length > 1) {
				await reread
			}
			const served = [birthDate, shoeSize]
			return HttpResponse.json({ data: { fields: served, every: served, reservedFieldNames: RESERVED } })
		}),
		graphql.mutation('OrderFields', () => HttpResponse.json(refusal('CONFLICT', 'field_order_incomplete'))),
	)

	renderScreen()
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The field list just changed. The new order was not saved.',
	)
	await waitFor(() => expect(reads).toHaveBeenCalledTimes(2))
	expect(await shownLabels()).toEqual(['Birth date', 'Shoe size'])
	releaseRead()
})

test('a refused order reads the catalogue again', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	server.use(graphql.mutation('OrderFields', () => HttpResponse.json(refusal('CONFLICT', 'field_order_incomplete'))))
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))
	await screen.findByRole('alert')

	expect(graph.refetch).toHaveBeenCalledWith(['FieldCatalogue'])
})

test('an order lost on the way puts the rows back without reading the catalogue again', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	server.use(graphql.mutation('OrderFields', () => HttpResponse.error()))
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await userEvent.click(await screen.findByRole('button', { name: 'Move Birth date down' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('The fields could not be ordered.')
	expect(await shownLabels()).toEqual(['Birth date', 'Shoe size'])
	expect(graph.refetch).not.toHaveBeenCalled()
})

test('the pressed arrow keeps focus and scrolls into view as its row moves', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	const ordered = hold('OrderFields', accepted)

	renderScreen()
	const down = await screen.findByRole('button', { name: 'Move Birth date down' })
	await userEvent.click(down)

	expect(await shownLabels()).toEqual(['Shoe size', 'Birth date'])
	expect(down).toHaveFocus()
	expect(scrolled).toHaveBeenCalledTimes(1)
	expect(scrolled.mock.contexts[0]).toBe(down)
	expect(scrolled).toHaveBeenCalledWith({ block: 'nearest' })
	ordered.release()
})

test('focus outside the list stays put and scrolls nothing as the rows change', async () => {
	let served = [birthDate]
	server.use(
		graphql.query('FieldCatalogue', () =>
			HttpResponse.json({ data: { fields: served, every: served, reservedFieldNames: RESERVED } }),
		),
		graphql.mutation('DefineField', () => {
			served = [shoeSize, birthDate]
			return HttpResponse.json({ data: { defineField: shoeSize } })
		}),
	)

	renderScreen()
	await defineLabelled('Shoe size')

	await waitFor(async () => expect(await shownLabels()).toEqual(['Shoe size', 'Birth date']))
	expect(screen.getByRole('button', { name: 'Add field' })).toHaveFocus()
	expect(scrolled).not.toHaveBeenCalled()
	expect(screen.getByText('Field added.')).toBeInTheDocument()
})

test('a second press on Archive before the archive is answered sends nothing more', async () => {
	serveFieldCatalogue([birthDate, shoeSize])
	const archived = vi.fn()
	let answer = () => {}
	const answered = new Promise<void>((resolve) => {
		answer = resolve
	})
	server.use(
		graphql.mutation('ArchiveField', async ({ variables }) => {
			archived(variables)
			if (archived.mock.calls.length > 1) {
				return HttpResponse.json(notFound)
			}
			await answered
			return HttpResponse.json({ data: { archiveField: true } })
		}),
	)
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await userEvent.click(await screen.findByRole('button', { name: 'Archive Birth date' }))
	const confirm = screen.getByRole('button', { name: 'Archive' })
	await userEvent.click(confirm)
	await userEvent.click(confirm)
	await waitFor(() => expect(archived).toHaveBeenCalledTimes(1))
	expect(confirm).toHaveAttribute('aria-disabled', 'true')
	expect(confirm.className).toMatch(/is-loading/)
	expect(screen.getByRole('button', { name: 'Keep' })).toHaveAttribute('aria-disabled', 'true')
	expect(screen.getByRole('button', { name: 'Archive Shoe size' })).toHaveAttribute('aria-disabled', 'true')
	await settle()
	answer()

	await waitFor(() => expect(graph.refetch).toHaveBeenCalledWith(['FieldCatalogue']))
	await settle()
	expect(archived).toHaveBeenCalledTimes(1)
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('the archive question stays open and locked until the read drops the row', async () => {
	let served = [birthDate, shoeSize, nickname]
	let gated = false
	let releaseRead = () => {}
	const reread = new Promise<void>((resolve) => {
		releaseRead = resolve
	})
	server.use(
		graphql.query('FieldCatalogue', async () => {
			if (gated) {
				await reread
			}
			return HttpResponse.json({ data: { fields: served, every: served, reservedFieldNames: RESERVED } })
		}),
		graphql.mutation('ArchiveField', ({ variables }) => {
			served = served.filter((field) => field.id !== variables.id)
			gated = true
			return HttpResponse.json({ data: { archiveField: true } })
		}),
	)
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await archiveField('Shoe size')
	await waitFor(() => expect(graph.refetch).toHaveBeenCalledWith(['FieldCatalogue']))
	await settle()

	const row = within(rowOf('Shoe size'))
	expect(row.getByRole('group', { name: 'Archive this field?' })).toBeInTheDocument()
	expect(row.getByRole('button', { name: 'Archive' })).toHaveAttribute('aria-disabled', 'true')
	expect(row.getByRole('button', { name: 'Keep' })).toHaveAttribute('aria-disabled', 'true')
	expect(screen.getByRole('button', { name: 'Archive Nickname' })).toHaveAttribute('aria-disabled', 'true')
	releaseRead()
	await waitFor(() => expect(screen.getByRole('button', { name: 'Archive Nickname' })).toHaveFocus())
})

test('an archive lost on the way can be confirmed again', async () => {
	serveFieldCatalogue([birthDate])
	const archived = vi.fn()
	server.use(
		graphql.mutation('ArchiveField', ({ variables }) => {
			archived(variables)
			return HttpResponse.error()
		}),
	)

	renderScreen()
	await archiveField('Birth date')
	expect(await screen.findByRole('alert')).toHaveTextContent('The field could not be archived.')
	await archiveField('Birth date')

	await waitFor(() => expect(archived).toHaveBeenCalledTimes(2))
})

test.each([
	['the next row', 'Shoe size', 'Archive Nickname'],
	['the row above when the last row goes', 'Nickname', 'Archive Shoe size'],
])('after an archive focus moves to the trash of %s', async (_, archived, next) => {
	serveFieldStore([birthDate, shoeSize, nickname])

	renderScreen()
	await archiveField(archived)
	await waitFor(() => expect(screen.queryByText(archived)).not.toBeInTheDocument())

	const trash = screen.getByRole('button', { name: next })
	expect(trash).toHaveFocus()
	expect(trash).not.toHaveAttribute('aria-disabled', 'true')
	expect(screen.getByText('Field archived.')).toBeInTheDocument()
})

test('a row a read drops hands focus to the same arrow in the next row', async () => {
	const store = serveFieldStore([birthDate, shoeSize, nickname])
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	const down = await screen.findByRole('button', { name: 'Move Shoe size down' })
	act(() => down.focus())
	store.serve([birthDate, nickname])
	act(() => graph.refetch(['FieldCatalogue']))
	await waitFor(() => expect(screen.queryByText('Shoe size')).not.toBeInTheDocument())

	expect(screen.getByRole('button', { name: 'Move Nickname down' })).toHaveFocus()
})

test('after the last field is archived focus moves to the list region', async () => {
	serveFieldStore([birthDate])

	renderScreen()
	await archiveField('Birth date')

	expect(await screen.findByText(/No fields yet/i)).toBeInTheDocument()
	const region = screen.getByRole('region', { name: 'Fields' })
	expect(region).toHaveFocus()
	expect(region).toHaveAttribute('tabindex', '-1')
	expect(screen.getByText('Field archived.')).toBeInTheDocument()
})

test('a field brought back after the reader archived it shows again', async () => {
	const store = serveFieldStore([birthDate, shoeSize])

	renderScreen()
	await archiveField('Birth date')
	await waitFor(() => expect(screen.queryByText('Birth date')).not.toBeInTheDocument())
	store.serve([shoeSize, birthDate])
	await defineLabelled('Hat size')

	await waitFor(async () => expect(await shownLabels()).toEqual(['Shoe size', 'Birth date', 'Hat size']))
})

test('a field that leaves and comes back after Keep leaves focus where the reader is', async () => {
	const store = serveFieldStore([birthDate, shoeSize])
	const { graph } = fakeGraphClient()

	renderScreen(graph)
	await userEvent.click(await screen.findByRole('button', { name: 'Archive Birth date' }))
	await userEvent.click(screen.getByRole('button', { name: 'Keep' }))
	store.serve([shoeSize])
	act(() => graph.refetch(['FieldCatalogue']))
	await waitFor(() => expect(screen.queryByText('Birth date')).not.toBeInTheDocument())
	store.serve([shoeSize, birthDate])
	await defineLabelled('Hat size')
	await waitFor(async () => expect(await shownLabels()).toEqual(['Shoe size', 'Birth date', 'Hat size']))
	await settle()

	expect(screen.getByRole('button', { name: 'Add field' })).toHaveFocus()
})

test('a list holding fields is one tab stop so a keyboard can scroll it', async () => {
	serveFieldCatalogue([birthDate])

	renderScreen()
	await screen.findByRole('button', { name: 'Archive Birth date' })

	expect(screen.getByRole('region', { name: 'Fields' })).toHaveAttribute('tabindex', '0')
})

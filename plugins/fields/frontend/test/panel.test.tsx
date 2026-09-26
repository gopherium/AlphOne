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

import { ContactFieldsPanel } from '../ContactFieldsPanel'

const contactID = '0198c000-0000-7000-8000-000000000401'

const birthDate = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000501',
	name: 'birthDate',
	label: 'Birth date',
	kind: 'DATE',
	subFields: [],
}

function renderPanel() {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	const { graph } = fakeGraphClient()
	return render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<ContactFieldsPanel contactId={contactID} />
			</GraphProvider>
		</QueryClientProvider>,
	)
}

function serveCatalogue(fields: unknown[]) {
	server.use(graphql.query('Fields', () => HttpResponse.json({ data: { fields } })))
}

function serveValues(values: Record<string, unknown>) {
	server.use(
		graphql.query('ContactFieldValues', () =>
			HttpResponse.json({
				data: { contact: { __typename: 'Contact', id: contactID, ...values } },
			}),
		),
	)
}

test('a defined field renders with its stored value', async () => {
	serveCatalogue([birthDate])
	serveValues({ birthDate: '1990-04-17' })

	renderPanel()

	const input = await screen.findByLabelText('Birth date')
	await waitFor(() => expect(input).toHaveValue('1990-04-17'))
})

test('a contact with no defined fields renders nothing', async () => {
	serveCatalogue([])

	const { container } = renderPanel()

	await waitFor(() => expect(container).toBeEmptyDOMElement())
})

test('a defined field with no value renders empty', async () => {
	serveCatalogue([birthDate])
	serveValues({ birthDate: null })

	renderPanel()

	expect(await screen.findByLabelText('Birth date')).toHaveValue('')
})

test('saving sends the edited value under its field name', async () => {
	serveCatalogue([birthDate])
	serveValues({ birthDate: null })
	const written = vi.fn()
	server.use(
		graphql.mutation('WriteContactFields', async ({ variables }) => {
			written(variables)
			return HttpResponse.json({ data: { writeContactFields: true } })
		}),
	)

	renderPanel()
	await userEvent.type(await screen.findByLabelText('Birth date'), '1990-04-17')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { birthDate: '1990-04-17' },
		}),
	)
})

test('a refused save is reported', async () => {
	serveCatalogue([birthDate])
	serveValues({ birthDate: null })
	server.use(
		graphql.mutation('WriteContactFields', () =>
			HttpResponse.json({
				errors: [
					{
						message: 'fields: birthDate expects DATE',
						extensions: { code: 'VALIDATION' },
					},
				],
			}),
		),
	)

	renderPanel()
	await userEvent.type(await screen.findByLabelText('Birth date'), 'nope')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(/expects DATE/)
})

const loyaltyPoints = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000502',
	name: 'loyaltyPoints',
	label: 'Loyalty points',
	kind: 'NUMBER',
	subFields: [],
}

const subscribed = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000503',
	name: 'subscribed',
	label: 'Subscribed',
	kind: 'BOOLEAN',
	subFields: [],
}

function captureWrite() {
	const written = vi.fn()
	server.use(
		graphql.mutation('WriteContactFields', async ({ variables }) => {
			written(variables)
			return HttpResponse.json({ data: { writeContactFields: true } })
		}),
	)
	return written
}

const jobTitle = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000504',
	name: 'jobTitle',
	label: 'Job title',
	kind: 'TEXT',
	subFields: [],
}

test('a text field sends its text unchanged', async () => {
	serveCatalogue([jobTitle])
	serveValues({ jobTitle: null })
	const written = captureWrite()

	renderPanel()
	await userEvent.type(await screen.findByLabelText('Job title'), 'Rear Admiral')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { jobTitle: 'Rear Admiral' },
		}),
	)
})

const notes = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000505',
	name: 'notes',
	label: 'Notes',
	kind: 'LONGTEXT',
	subFields: [],
}

test('a long text field renders a text area with its stored text', async () => {
	serveCatalogue([notes])
	serveValues({ notes: 'First line\nSecond line' })

	renderPanel()

	const area = await screen.findByRole('textbox', { name: 'Notes' })
	expect(area).toBeInstanceOf(HTMLTextAreaElement)
	expect(area).toHaveAttribute('rows', '4')
	await waitFor(() => expect(area).toHaveValue('First line\nSecond line'))
})

test('a long text field keeps the line breaks it is given', async () => {
	serveCatalogue([notes])
	serveValues({ notes: null })
	const written = captureWrite()

	renderPanel()
	await userEvent.type(await screen.findByLabelText('Notes'), 'First line{Enter}Second line')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { notes: 'First line\nSecond line' },
		}),
	)
	expect(written).toHaveBeenCalledTimes(1)
})

test('a number field sends a number, not its text', async () => {
	serveCatalogue([loyaltyPoints])
	serveValues({ loyaltyPoints: null })
	const written = captureWrite()

	renderPanel()
	await userEvent.type(await screen.findByLabelText('Loyalty points'), '420')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { loyaltyPoints: 420 },
		}),
	)
})

test('a boolean field renders a checkbox and sends a boolean', async () => {
	serveCatalogue([subscribed])
	serveValues({ subscribed: false })
	const written = captureWrite()

	renderPanel()
	await userEvent.click(await screen.findByLabelText('Subscribed'))
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { subscribed: true },
		}),
	)
})

test('unchecking a boolean field sends false', async () => {
	serveCatalogue([subscribed])
	serveValues({ subscribed: true })
	const written = captureWrite()

	renderPanel()
	const box = await screen.findByLabelText('Subscribed')
	await waitFor(() => expect(box).toBeChecked())
	await userEvent.click(box)
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { subscribed: false },
		}),
	)
})

test('a boolean field shows its label on screen', async () => {
	serveCatalogue([subscribed])
	serveValues({ subscribed: false })

	renderPanel()

	const box = await screen.findByLabelText('Subscribed')
	expect(box).toBeInTheDocument()
	expect(screen.getByText('Subscribed')).toBeVisible()
})

test('a boolean field shows its stored value', async () => {
	serveCatalogue([subscribed])
	serveValues({ subscribed: true })

	renderPanel()

	await waitFor(() => expect(screen.getByLabelText('Subscribed')).toBeChecked())
})

test('an untouched field is left out of the write', async () => {
	serveCatalogue([birthDate, loyaltyPoints])
	serveValues({ birthDate: '1990-04-17', loyaltyPoints: 420 })
	const written = captureWrite()

	renderPanel()
	const input = await screen.findByLabelText('Loyalty points')
	await waitFor(() => expect(input).toHaveValue(420))
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() => expect(written).toHaveBeenCalledWith({ contactId: contactID, values: {} }))
})

test('clearing a field sends null', async () => {
	serveCatalogue([birthDate])
	serveValues({ birthDate: '1990-04-17' })
	const written = captureWrite()

	renderPanel()
	const input = await screen.findByLabelText('Birth date')
	await waitFor(() => expect(input).toHaveValue('1990-04-17'))
	await userEvent.clear(input)
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { birthDate: null },
		}),
	)
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

const firstCall = { date: '2026-09-01', comment: 'First call about the yearly plan.' }

const offerSent = { date: '2026-09-10', comment: 'Sent the offer and booked a follow-up call.' }

async function entry(number: number) {
	return within(await screen.findByRole('group', { name: `History ${number}` }))
}

test('a repeater renders one group per stored entry, long text in a text area', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall, offerSent] })

	renderPanel()

	const first = await entry(1)
	expect(first.getByLabelText('Date')).toHaveValue('2026-09-01')
	expect(first.getByRole('textbox', { name: 'Comment' })).toBeInstanceOf(HTMLTextAreaElement)
	expect(first.getByRole('textbox', { name: 'Comment' })).toHaveValue(firstCall.comment)
	expect((await entry(2)).getByLabelText('Date')).toHaveValue('2026-09-10')
	expect(screen.getByRole('group', { name: 'History' })).toBeInTheDocument()
})

test('a repeater holding no entries invites the first one', async () => {
	serveCatalogue([history])
	serveValues({ history: null })

	renderPanel()

	expect(await screen.findByText('No entries yet.')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Add an entry to History' })).toBeInTheDocument()
})

test('a stored entry missing a cell renders that cell empty', async () => {
	serveCatalogue([history])
	serveValues({ history: [{ comment: firstCall.comment }] })

	renderPanel()

	expect((await entry(1)).getByLabelText('Date')).toHaveValue('')
})

test('adding an entry sends the whole list with the new entry last', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall] })
	const written = captureWrite()

	renderPanel()
	await entry(1)
	await userEvent.click(screen.getByRole('button', { name: 'Add an entry to History' }))
	const added = await entry(2)
	await userEvent.type(added.getByLabelText('Date'), '2026-09-20')
	await userEvent.type(added.getByLabelText('Comment'), 'Called back.{Enter}Agreed on a date.')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: {
				history: [firstCall, { date: '2026-09-20', comment: 'Called back.\nAgreed on a date.' }],
			},
		}),
	)
	expect(written).toHaveBeenCalledTimes(1)
})

test('a blank cell in an entry is sent as null', async () => {
	serveCatalogue([history])
	serveValues({ history: null })
	const written = captureWrite()

	renderPanel()
	await userEvent.click(await screen.findByRole('button', { name: 'Add an entry to History' }))
	await userEvent.type((await entry(1)).getByLabelText('Comment'), 'Left a message.')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { history: [{ date: null, comment: 'Left a message.' }] },
		}),
	)
})

test('moving an entry up sends the list in its new order', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall, offerSent] })
	const written = captureWrite()

	renderPanel()
	await userEvent.click((await entry(2)).getByRole('button', { name: 'Move entry up' }))
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { history: [offerSent, firstCall] },
		}),
	)
})

test('removing the only entry sends null', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall] })
	const written = captureWrite()

	renderPanel()
	await userEvent.click((await entry(1)).getByRole('button', { name: 'Remove entry' }))
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({ contactId: contactID, values: { history: null } }),
	)
})

test('an untouched repeater is left out of the write', async () => {
	serveCatalogue([history, jobTitle])
	serveValues({ history: [firstCall], jobTitle: null })
	const written = captureWrite()

	renderPanel()
	await entry(1)
	await userEvent.type(screen.getByLabelText('Job title'), 'Rear Admiral')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { jobTitle: 'Rear Admiral' },
		}),
	)
})

test('a number cell in an entry sends a number', async () => {
	const visits = {
		__typename: 'FieldDefinition',
		id: '0198c000-0000-7000-8000-000000000507',
		name: 'visits',
		label: 'Visits',
		kind: 'REPEATER',
		subFields: [{ __typename: 'FieldSubField', name: 'minutes', label: 'Minutes', kind: 'NUMBER' }],
	}
	serveCatalogue([visits])
	serveValues({ visits: null })
	const written = captureWrite()

	renderPanel()
	await userEvent.click(await screen.findByRole('button', { name: 'Add an entry to Visits' }))
	await userEvent.type(
		within(screen.getByRole('group', { name: 'Visits 1' })).getByLabelText('Minutes'),
		'45',
	)
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { visits: [{ minutes: 45 }] },
		}),
	)
})

test('a saved entry list is edited afresh from what the graph answers', async () => {
	serveCatalogue([history])
	let stored: unknown = [firstCall]
	server.use(
		graphql.query('ContactFieldValues', () =>
			HttpResponse.json({
				data: { contact: { __typename: 'Contact', id: contactID, history: stored } },
			}),
		),
		graphql.mutation('WriteContactFields', () => {
			stored = [offerSent]
			return HttpResponse.json({ data: { writeContactFields: true } })
		}),
	)

	renderPanel()
	await entry(1)
	await userEvent.click(screen.getByRole('button', { name: 'Add an entry to History' }))
	await entry(2)
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(async () => expect((await entry(1)).getByLabelText('Date')).toHaveValue(offerSent.date))
	expect(screen.queryByRole('group', { name: 'History 2' })).not.toBeInTheDocument()
})

test('unsaved entries stay with the contact they were typed on', async () => {
	serveCatalogue([history])
	const otherID = '0198c000-0000-7000-8000-000000000402'
	server.use(
		graphql.query('ContactFieldValues', ({ variables }) =>
			HttpResponse.json({
				data: {
					contact: {
						__typename: 'Contact',
						id: variables.id,
						history: variables.id === contactID ? [firstCall] : [offerSent],
					},
				},
			}),
		),
	)
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	const { graph } = fakeGraphClient()
	const shown = (id: string) => (
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<ContactFieldsPanel contactId={id} />
			</GraphProvider>
		</QueryClientProvider>
	)

	const { rerender } = render(shown(contactID))
	await entry(1)
	await userEvent.click(screen.getByRole('button', { name: 'Add an entry to History' }))
	await entry(2)
	rerender(shown(otherID))

	await waitFor(async () => expect((await entry(1)).getByLabelText('Date')).toHaveValue(offerSent.date))
	expect(screen.queryByRole('group', { name: 'History 2' })).not.toBeInTheDocument()
})

test('the editor waits for the stored entries before it opens', async () => {
	serveCatalogue([history])
	let release = () => {}
	const answered = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		graphql.query('ContactFieldValues', async () => {
			await answered
			return HttpResponse.json({
				data: { contact: { __typename: 'Contact', id: contactID, history: [firstCall] } },
			})
		}),
	)

	renderPanel()

	expect(await screen.findByRole('status')).toHaveTextContent('Loading fields…')
	expect(screen.queryByRole('button', { name: 'Add an entry to History' })).not.toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Save fields' })).not.toBeInTheDocument()
	release()
	expect((await entry(1)).getByLabelText('Date')).toHaveValue(firstCall.date)
})

test('a contact the graph no longer finds opens with no stored entries', async () => {
	serveCatalogue([history])
	server.use(graphql.query('ContactFieldValues', () => HttpResponse.json({ data: { contact: null } })))

	renderPanel()

	expect(await screen.findByText('No entries yet.')).toBeInTheDocument()
})

test('a failed value read is reported instead of an empty editor', async () => {
	serveCatalogue([history])
	server.use(
		graphql.query('ContactFieldValues', () => HttpResponse.json({ errors: [{ message: 'boom' }] })),
	)

	renderPanel()

	expect(await screen.findByRole('alert')).toHaveTextContent('The fields could not be loaded.')
	expect(screen.queryByRole('button', { name: 'Add an entry to History' })).not.toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Save fields' })).not.toBeInTheDocument()
})

test('a failed catalogue read leaves the contact screen alone', async () => {
	server.use(
		graphql.query('Fields', () => HttpResponse.json({ errors: [{ message: 'boom' }] })),
	)

	const { container } = renderPanel()

	await waitFor(() => expect(container).toBeEmptyDOMElement())
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

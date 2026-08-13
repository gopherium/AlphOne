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
import { expect, test, vi } from 'vitest'

import { ContactFieldsPanel } from '../ContactFieldsPanel'

const contactID = '0198c000-0000-7000-8000-000000000401'

const birthDate = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000501',
	name: 'birthDate',
	label: 'Birth date',
	kind: 'DATE',
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
}

const subscribed = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000503',
	name: 'subscribed',
	label: 'Subscribed',
	kind: 'BOOLEAN',
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

test('a failed catalogue read leaves the contact screen alone', async () => {
	server.use(
		graphql.query('Fields', () => HttpResponse.json({ errors: [{ message: 'boom' }] })),
	)

	const { container } = renderPanel()

	await waitFor(() => expect(container).toBeEmptyDOMElement())
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

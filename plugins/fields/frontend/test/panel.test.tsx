// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import {
	captureWrite,
	contactID,
	history,
	hold,
	jobTitle,
	renderPanel,
	serveCatalogue,
	serveValues,
	visits,
} from './harness'

const birthDate = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000501',
	name: 'birthDate',
	label: 'Birth date',
	kind: 'DATE',
	subFields: [],
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

	const { slot } = renderPanel()

	await waitFor(() => expect(slot).toBeEmptyDOMElement())
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
	expect(await screen.findByText('Fields saved.')).toBeInTheDocument()
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
	await userEvent.type(await screen.findByLabelText('Birth date'), '1990-04-17')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(/expects DATE/)
	expect(screen.queryByText('Fields saved.')).not.toBeInTheDocument()
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
	await userEvent.clear(input)
	await userEvent.type(input, '421')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))

	await waitFor(() => expect(written).toHaveBeenCalledWith({ contactId: contactID, values: { loyaltyPoints: 421 } }))
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

test('a failed value read is reported instead of an empty editor', async () => {
	serveCatalogue([history, jobTitle])
	server.use(
		graphql.query('ContactFieldValues', () => HttpResponse.json({ errors: [{ message: 'boom' }] })),
	)

	renderPanel()

	expect(await screen.findByRole('alert')).toHaveTextContent('The fields could not be loaded.')
	expect(screen.queryByRole('button', { name: 'Add an entry to History' })).not.toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Save fields' })).not.toBeInTheDocument()
})

/**
 * Returns the Save fields button of the form holding the named field.
 * @param label - The label of a plain field.
 * @returns The button.
 */
function saveOf(label: string) {
	const form = screen.getByLabelText(label).closest('form') as HTMLElement
	return within(form).getByRole('button', { name: 'Save fields' })
}

test('shows every field in the order the server lists them, each run of plain fields in its own form', async () => {
	serveCatalogue([birthDate, history, jobTitle, loyaltyPoints, visits])
	serveValues({ birthDate: null, history: null, jobTitle: null, loyaltyPoints: null, visits: null })

	renderPanel()

	const date = await screen.findByLabelText('Birth date')
	const saves = screen.getAllByRole('button', { name: 'Save fields' })
	expect(saves).toHaveLength(2)
	const shown = [
		date,
		saves[0],
		screen.getByRole('group', { name: 'History' }),
		screen.getByLabelText('Job title'),
		screen.getByLabelText('Loyalty points'),
		saves[1],
		screen.getByRole('group', { name: 'Visits' }),
	]
	const placed = [...shown].sort((one, other) =>
		one.compareDocumentPosition(other) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1,
	)
	expect(placed.map((element) => shown.indexOf(element))).toEqual([0, 1, 2, 3, 4, 5, 6])
	expect(saveOf('Birth date')).toBe(saves[0])
	expect(saveOf('Job title')).toBe(saves[1])
	expect(saveOf('Loyalty points')).toBe(saves[1])
})

test('a Save fields writes the edits of every run', async () => {
	serveCatalogue([birthDate, history, jobTitle])
	serveValues({ birthDate: null, history: null, jobTitle: null })
	const written = captureWrite()

	renderPanel()
	await userEvent.type(await screen.findByLabelText('Birth date'), '1990-04-17')
	await userEvent.type(screen.getByLabelText('Job title'), 'Rear Admiral')
	await userEvent.click(saveOf('Job title'))

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({
			contactId: contactID,
			values: { birthDate: '1990-04-17', jobTitle: 'Rear Admiral' },
		}),
	)
	expect(written).toHaveBeenCalledTimes(1)
})

test('a Save fields stays off until a field of its own run is changed', async () => {
	serveCatalogue([birthDate, history, jobTitle])
	serveValues({ birthDate: null, history: null, jobTitle: null })
	const written = captureWrite()

	renderPanel()
	await screen.findByLabelText('Birth date')
	expect(saveOf('Birth date')).toHaveAttribute('aria-disabled', 'true')
	expect(saveOf('Job title')).toHaveAttribute('aria-disabled', 'true')
	await userEvent.click(saveOf('Job title'))
	await userEvent.type(screen.getByLabelText('Job title'), 'R')

	expect(saveOf('Birth date')).toHaveAttribute('aria-disabled', 'true')
	expect(saveOf('Job title')).not.toHaveAttribute('aria-disabled', 'true')
	expect(written).not.toHaveBeenCalled()
})

test('a save clears the edits of every run and shows the values read again', async () => {
	serveCatalogue([birthDate, history, jobTitle])
	const reads = vi.fn()
	server.use(
		graphql.query('ContactFieldValues', () => {
			reads()
			const saved = reads.mock.calls.length > 1
			return HttpResponse.json({
				data: {
					contact: {
						__typename: 'Contact',
						id: contactID,
						birthDate: saved ? '1990-04-17' : null,
						history: null,
						jobTitle: saved ? 'Rear Admiral' : null,
					},
				},
			})
		}),
	)
	const written = captureWrite()

	renderPanel()
	await userEvent.type(await screen.findByLabelText('Job title'), 'Rear Admiral')
	await userEvent.type(screen.getByLabelText('Birth date'), '1990-04-17')
	await userEvent.click(saveOf('Birth date'))

	await waitFor(() => expect(reads).toHaveBeenCalledTimes(2))
	expect(written).toHaveBeenCalledWith({
		contactId: contactID,
		values: { birthDate: '1990-04-17', jobTitle: 'Rear Admiral' },
	})
	await waitFor(() => expect(saveOf('Birth date')).toHaveAttribute('aria-disabled', 'true'))
	expect(saveOf('Job title')).toHaveAttribute('aria-disabled', 'true')
	expect(screen.getByLabelText('Birth date')).toHaveValue('1990-04-17')
	expect(screen.getByLabelText('Job title')).toHaveValue('Rear Admiral')
})

test.each([
	{ label: 'Birth date', text: '1990-04-17' },
	{ label: 'Job title', text: 'Rear Admiral' },
])('a refused save from the $label run shows one notice inside that form', async ({ label, text }) => {
	serveCatalogue([birthDate, history, jobTitle])
	serveValues({ birthDate: null, history: null, jobTitle: null })
	server.use(
		graphql.mutation('WriteContactFields', () =>
			HttpResponse.json({ errors: [{ message: 'fields: birthDate expects DATE', extensions: { code: 'VALIDATION' } }] }),
		),
	)

	renderPanel()
	await userEvent.type(await screen.findByLabelText(label), text)
	await userEvent.click(saveOf(label))

	const notice = await screen.findByRole('alert')
	expect(screen.getAllByRole('alert')).toHaveLength(1)
	expect(saveOf(label).closest('form')).toContainElement(notice)
})

test('a refused save keeps its notice in the pressed run when the repeater before it goes', async () => {
	serveCatalogue([birthDate, history, jobTitle])
	serveValues({ birthDate: null, history: null, jobTitle: null })
	server.use(
		graphql.mutation('WriteContactFields', () =>
			HttpResponse.json({ errors: [{ message: 'fields: jobTitle refused', extensions: { code: 'VALIDATION' } }] }),
		),
	)

	const { graph } = renderPanel()
	await userEvent.type(await screen.findByLabelText('Job title'), 'Rear Admiral')
	await userEvent.click(saveOf('Job title'))
	await screen.findByRole('alert')
	serveCatalogue([birthDate, jobTitle])
	serveValues({ birthDate: null, jobTitle: null })
	await act(async () => {
		graph.refetch(['Fields'])
	})

	await waitFor(() => expect(screen.queryByRole('group', { name: 'History' })).not.toBeInTheDocument())
	await waitFor(() => expect(saveOf('Job title').closest('form')).toContainElement(screen.getByRole('alert')))
})

test('a save shows its spinner only on the Save fields pressed and keeps every Save fields off', async () => {
	serveCatalogue([birthDate, history, jobTitle])
	serveValues({ birthDate: null, history: null, jobTitle: null })
	const held = hold('WriteContactFields', { data: { writeContactFields: true } })

	renderPanel()
	await userEvent.type(await screen.findByLabelText('Birth date'), '1990-04-17')
	await userEvent.type(screen.getByLabelText('Job title'), 'Rear Admiral')
	await userEvent.click(saveOf('Birth date'))
	await waitFor(() => expect(held.called).toHaveBeenCalledOnce())

	expect(saveOf('Birth date').className).toMatch(/is-loading/)
	expect(saveOf('Job title').className).not.toMatch(/is-loading/)
	expect(saveOf('Birth date')).toHaveAttribute('aria-disabled', 'true')
	expect(saveOf('Job title')).toHaveAttribute('aria-disabled', 'true')
	held.release()
	await waitFor(() => expect(saveOf('Birth date').className).not.toMatch(/is-loading/))
})

test('a Save fields stays off while its save runs and a second press sends nothing', async () => {
	serveCatalogue([jobTitle])
	serveValues({ jobTitle: null })
	const held = hold('WriteContactFields', { data: { writeContactFields: true } })

	const { graph } = renderPanel()
	await userEvent.type(await screen.findByLabelText('Job title'), 'Rear Admiral')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))
	await waitFor(() => expect(held.called).toHaveBeenCalledOnce())

	expect(screen.getByRole('button', { name: 'Save fields' })).toHaveAttribute('aria-disabled', 'true')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))
	held.release()
	await waitFor(() => expect(graph.refetch).toHaveBeenCalledWith(['ContactFieldValues']))
	expect(held.called).toHaveBeenCalledOnce()
})

test('text typed in another run while a save runs stays and the next Save fields writes only it', async () => {
	serveCatalogue([birthDate, history, jobTitle])
	serveValues({ birthDate: null, history: null, jobTitle: null })
	const held = hold('WriteContactFields', { data: { writeContactFields: true } })

	const { graph } = renderPanel()
	await userEvent.type(await screen.findByLabelText('Birth date'), '1990-04-17')
	await userEvent.click(saveOf('Birth date'))
	await waitFor(() => expect(held.called).toHaveBeenCalledOnce())
	await userEvent.type(screen.getByLabelText('Job title'), 'Rear Admiral')
	held.release()
	await waitFor(() => expect(graph.refetch).toHaveBeenCalledWith(['ContactFieldValues']))

	expect(screen.getByLabelText('Job title')).toHaveValue('Rear Admiral')
	expect(saveOf('Job title')).not.toHaveAttribute('aria-disabled', 'true')
	expect(saveOf('Birth date')).toHaveAttribute('aria-disabled', 'true')
	await userEvent.click(saveOf('Job title'))
	await waitFor(() => expect(held.called).toHaveBeenCalledTimes(2))
	expect(held.called).toHaveBeenLastCalledWith({ contactId: contactID, values: { jobTitle: 'Rear Admiral' } })
})

test('a field typed on while its save runs keeps the new text and its Save fields on', async () => {
	serveCatalogue([jobTitle])
	serveValues({ jobTitle: null })
	const held = hold('WriteContactFields', { data: { writeContactFields: true } })

	const { graph } = renderPanel()
	const input = await screen.findByLabelText('Job title')
	await userEvent.type(input, 'Rear Admiral')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))
	await waitFor(() => expect(held.called).toHaveBeenCalledOnce())
	await userEvent.type(input, ' Retired')
	held.release()
	await waitFor(() => expect(graph.refetch).toHaveBeenCalledWith(['ContactFieldValues']))

	expect(input).toHaveValue('Rear Admiral Retired')
	expect(screen.getByRole('button', { name: 'Save fields' })).not.toHaveAttribute('aria-disabled', 'true')
	await userEvent.click(screen.getByRole('button', { name: 'Save fields' }))
	await waitFor(() => expect(held.called).toHaveBeenCalledTimes(2))
	expect(held.called).toHaveBeenLastCalledWith({ contactId: contactID, values: { jobTitle: 'Rear Admiral Retired' } })
})

test('a failed catalogue read leaves the contact screen alone', async () => {
	server.use(
		graphql.query('Fields', () => HttpResponse.json({ errors: [{ message: 'boom' }] })),
	)

	const { slot } = renderPanel()

	await waitFor(() => expect(slot).toBeEmptyDOMElement())
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { configureErrorText } from '@alphone/frontend-sdk'
import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import {
	ID1,
	ID2,
	ID3,
	capture,
	captureWrite,
	contactID,
	firstButton,
	firstCall,
	followUp,
	history,
	hold,
	jobTitle,
	offerSent,
	otherContactID,
	refusal,
	renderPanel,
	serveCatalogue,
	serveChangingValues,
	serveValues,
	speakTemplates,
	visits,
} from './harness'

/** added is the body a successful add answers. */
const added = { data: { addContactFieldEntry: { id: ID3, comment: 'x' } } }

/** removed is the body a successful removal answers. */
const removed = { data: { deleteContactFieldEntry: true } }

/** now is the moment the add form's dates start from. */
const now = new Date()

/** today is the local calendar day the add form starts its dates on. */
const today = [
	String(now.getFullYear()).padStart(4, '0'),
	String(now.getMonth() + 1).padStart(2, '0'),
	String(now.getDate()).padStart(2, '0'),
].join('-')

afterEach(() => {
	configureErrorText({ templates: () => ({}), fallback: () => '' })
	vi.unstubAllEnvs()
})

/**
 * Returns the add form of the named repeater.
 * @param label - The repeater's label.
 * @returns The form, once it shows.
 */
async function addForm(label = 'History') {
	return within(await screen.findByRole('form', { name: `Add an entry to ${label}` }))
}

/**
 * Returns the add button of the named repeater.
 * @param label - The repeater's label.
 * @returns The button.
 */
function addButton(label = 'History') {
	return screen.getByRole('button', { name: `Add an entry to ${label}` })
}

/**
 * Returns the names of the items the named list shows, in order.
 * @param label - The repeater's label.
 * @returns The item names.
 */
async function itemNames(label = 'History') {
	const list = await screen.findByRole('list', { name: label })
	return within(list)
		.getAllByRole('listitem')
		.map((item) => item.getAttribute('aria-label'))
}

/**
 * Asks to remove the named entry and confirms it.
 * @param name - The entry's name.
 */
async function removeEntry(name: string) {
	await userEvent.click(await screen.findByRole('button', { name: `Remove entry: ${name}` }))
	await userEvent.click(screen.getByRole('button', { name: 'Remove' }))
}

test('lists entries in the order the graph answers them', async () => {
	serveCatalogue([history])
	serveValues({ history: [offerSent, firstCall] })

	renderPanel()

	expect(await itemNames()).toEqual(['Sep 10, 2026', 'Sep 1, 2026'])
})

test('shows the first date as a muted time above the text', async () => {
	serveCatalogue([history])
	serveValues({ history: [offerSent] })

	renderPanel()

	const item = await screen.findByRole('listitem', { name: 'Sep 10, 2026' })
	const time = item.querySelector('time')
	expect(time).toHaveAttribute('datetime', '2026-09-10')
	expect(time).toHaveTextContent('Sep 10, 2026')
	expect(item.querySelector('p.godmin-log-list__body')?.textContent).toBe(offerSent.comment)
})

test('keeps the line breaks of its text', async () => {
	serveCatalogue([history])
	serveValues({ history: [followUp] })

	renderPanel()

	const item = await screen.findByRole('listitem', { name: 'Sep 18, 2026' })
	expect(item.querySelector('p.godmin-log-list__body')?.textContent).toBe(followUp.comment)
})

test('shows other cells as label and value lines in sub field order', async () => {
	serveCatalogue([visits])
	serveValues({
		visits: [
			{
				id: ID1,
				date: '2026-09-10',
				note: 'Paid in cash.',
				minutes: 45,
				paid: true,
				channel: 'Phone',
				followUpOn: '2026-10-01',
			},
		],
	})

	renderPanel()

	const item = await screen.findByRole('listitem', { name: 'Sep 10, 2026' })
	expect([...item.querySelectorAll('p')].map((line) => line.textContent)).toEqual([
		'Paid in cash.',
		'Minutes: 45',
		'Paid: Yes',
		'Channel: Phone',
		'Follow up on: Oct 1, 2026',
	])
})

test('a false yes or no cell shows No', async () => {
	serveCatalogue([visits])
	serveValues({ visits: [{ id: ID1, paid: false }] })

	renderPanel()

	expect(await screen.findByText('Paid: No')).toBeInTheDocument()
})

test('leaves out cells that hold no value', async () => {
	serveCatalogue([history])
	serveValues({ history: [{ id: ID1, comment: 'Left a message.' }] })

	renderPanel()

	const item = await screen.findByRole('listitem', { name: 'Left a message.' })
	expect(item.querySelector('time')).toBeNull()
	expect(within(item).queryByText(/Date:/)).not.toBeInTheDocument()
})

test('a sub field named after a built-in member shows nothing inherited', async () => {
	serveCatalogue([
		{
			...history,
			subFields: [{ __typename: 'FieldSubField', name: 'constructor', label: 'Builder', kind: 'TEXT' }],
		},
	])
	serveValues({ history: [{ id: ID1 }] })

	renderPanel()

	const item = await screen.findByRole('listitem', { name: 'Blank entry' })
	expect(item).not.toHaveTextContent('function')
	expect((await addForm()).getByLabelText('Builder')).toHaveValue('')
})

test('names each row by its day, its first line or its first detail', async () => {
	serveCatalogue([history, visits])
	serveValues({
		history: [offerSent, { id: ID3, comment: 'Left a message.\nNo answer.' }],
		visits: [{ id: ID1, minutes: 45 }],
	})

	renderPanel()

	expect(await itemNames()).toEqual(['Sep 10, 2026', 'Left a message.'])
	expect(await itemNames('Visits')).toEqual(['Minutes: 45'])
	expect(screen.getByRole('button', { name: 'Remove entry: Sep 10, 2026' })).toBeInTheDocument()
})

test('shows the day an entry names to a reader west of UTC', async () => {
	vi.stubEnv('TZ', 'America/Los_Angeles')
	serveCatalogue([history])
	serveValues({ history: [offerSent] })

	renderPanel()

	const item = await screen.findByRole('listitem', { name: 'Sep 10, 2026' })
	expect(item.querySelector('time')).toHaveTextContent('Sep 10, 2026')
})

test('no entries invites the first one', async () => {
	serveCatalogue([history])
	serveValues({ history: null })

	renderPanel()

	expect(await screen.findByText('No entries yet.')).toHaveAttribute('role', 'status')
	expect(addButton()).toBeInTheDocument()
})

test('a contact the graph no longer finds shows no entries', async () => {
	serveCatalogue([history])
	server.use(graphql.query('ContactFieldValues', () => HttpResponse.json({ data: { contact: null } })))

	renderPanel()

	expect(await screen.findByText('No entries yet.')).toBeInTheDocument()
})

test('the add form starts its dates on today', async () => {
	serveCatalogue([history])
	serveValues({ history: null })

	renderPanel()

	const form = await addForm()
	expect(form.getByLabelText('Date')).toHaveValue(today)
	expect(form.getByLabelText('Comment')).toHaveValue('')
})

test('Add stays off on an untouched form holding only the dates it starts with', async () => {
	serveCatalogue([history])
	serveValues({ history: null })

	renderPanel()
	await addForm()

	expect(addButton()).toHaveAttribute('aria-disabled', 'true')
})

test('Add stays off while the typed cells hold only white space', async () => {
	serveCatalogue([history])
	serveValues({ history: null })

	renderPanel()
	const comment = (await addForm()).getByLabelText('Comment')
	await userEvent.type(comment, '   ')
	expect(addButton()).toHaveAttribute('aria-disabled', 'true')
	await userEvent.type(comment, 'x')

	expect(addButton()).not.toHaveAttribute('aria-disabled', 'true')
})

test('a picked date alone turns Add on', async () => {
	serveCatalogue([history])
	serveValues({ history: null })

	renderPanel()
	const date = (await addForm()).getByLabelText('Date')
	await userEvent.clear(date)
	await userEvent.type(date, '2026-09-20')

	expect(addButton()).not.toHaveAttribute('aria-disabled', 'true')
})

test('adding sends only that entry, never the field values', async () => {
	serveCatalogue([history])
	serveValues({ history: null })
	const adding = capture('AddContactFieldEntry', added)
	const written = captureWrite()

	renderPanel()
	await userEvent.type((await addForm()).getByLabelText('Comment'), 'Called back.{Enter}Agreed on a date.')
	await userEvent.click(addButton())

	await waitFor(() =>
		expect(adding).toHaveBeenCalledWith({
			contactId: contactID,
			field: 'history',
			entry: { date: today, comment: 'Called back.\nAgreed on a date.' },
		}),
	)
	expect(written).not.toHaveBeenCalled()
})

test('a picked date is sent in place of today', async () => {
	serveCatalogue([history])
	serveValues({ history: null })
	const adding = capture('AddContactFieldEntry', added)

	renderPanel()
	const form = await addForm()
	await userEvent.clear(form.getByLabelText('Date'))
	await userEvent.type(form.getByLabelText('Date'), '2026-09-20')
	await userEvent.type(form.getByLabelText('Comment'), 'x')
	await userEvent.click(addButton())

	await waitFor(() => expect(adding).toHaveBeenCalledOnce())
	expect(adding.mock.calls[0][0].entry.date).toBe('2026-09-20')
})

test('a cleared date is sent as null', async () => {
	serveCatalogue([history])
	serveValues({ history: null })
	const adding = capture('AddContactFieldEntry', added)

	renderPanel()
	const form = await addForm()
	await userEvent.clear(form.getByLabelText('Date'))
	await userEvent.type(form.getByLabelText('Comment'), 'Left a message.')
	await userEvent.click(addButton())

	await waitFor(() => expect(adding).toHaveBeenCalledOnce())
	expect(adding.mock.calls[0][0].entry).toEqual({ date: null, comment: 'Left a message.' })
})

test('a cell holding only white space is sent as null', async () => {
	serveCatalogue([history])
	serveValues({ history: null })
	const adding = capture('AddContactFieldEntry', added)

	renderPanel()
	const form = await addForm()
	await userEvent.clear(form.getByLabelText('Date'))
	await userEvent.type(form.getByLabelText('Date'), '2026-09-20')
	await userEvent.type(form.getByLabelText('Comment'), '  ')
	await userEvent.click(addButton())

	await waitFor(() => expect(adding).toHaveBeenCalledOnce())
	expect(adding.mock.calls[0][0].entry).toEqual({ date: '2026-09-20', comment: null })
})

test('a number cell is sent as a number', async () => {
	serveCatalogue([visits])
	serveValues({ visits: null })
	const adding = capture('AddContactFieldEntry', added)

	renderPanel()
	await userEvent.type((await addForm('Visits')).getByLabelText('Minutes'), '45')
	await userEvent.click(addButton('Visits'))

	await waitFor(() => expect(adding).toHaveBeenCalledOnce())
	expect(adding.mock.calls[0][0].entry).toEqual(expect.objectContaining({ minutes: 45, paid: null }))
})

test('a yes or no cell is sent as a boolean', async () => {
	serveCatalogue([visits])
	serveValues({ visits: null })
	const adding = capture('AddContactFieldEntry', added)

	renderPanel()
	await userEvent.click((await addForm('Visits')).getByRole('checkbox', { name: 'Paid' }))
	await userEvent.click(addButton('Visits'))

	await waitFor(() => expect(adding).toHaveBeenCalledOnce())
	expect(adding.mock.calls[0][0].entry).toEqual(expect.objectContaining({ paid: true }))
})

test('Add shows it is busy while the add runs', async () => {
	serveCatalogue([history])
	serveValues({ history: null })
	const adding = hold('AddContactFieldEntry', added)

	renderPanel()
	await userEvent.type((await addForm()).getByLabelText('Comment'), 'x')
	await userEvent.click(addButton())
	await waitFor(() => expect(adding.called).toHaveBeenCalledOnce())

	expect(addButton()).toHaveAttribute('aria-disabled', 'true')
	adding.release()
	await waitFor(() => expect(addButton()).toHaveAttribute('aria-disabled', 'true'))
})

test('an added entry resets the form to today and shows on top', async () => {
	serveCatalogue([history])
	const change = serveChangingValues({ history: [firstCall] })
	capture('AddContactFieldEntry', added)

	const { graph } = renderPanel()
	await screen.findByRole('listitem', { name: 'Sep 1, 2026' })
	change({ history: [followUp, firstCall] })
	const form = await addForm()
	await userEvent.clear(form.getByLabelText('Date'))
	await userEvent.type(form.getByLabelText('Date'), '2026-09-18')
	await userEvent.type(form.getByLabelText('Comment'), 'Follow-up call.')
	await userEvent.click(addButton())

	await waitFor(async () => expect((await itemNames())[0]).toBe('Sep 18, 2026'))
	expect(graph.refetch).toHaveBeenCalledWith(['ContactFieldValues'])
	expect(form.getByLabelText('Date')).toHaveValue(today)
	expect(form.getByLabelText('Comment')).toHaveValue('')
})

test('an add returns focus to the first cell', async () => {
	serveCatalogue([history])
	serveValues({ history: null })
	capture('AddContactFieldEntry', added)

	renderPanel()
	const form = await addForm()
	await userEvent.type(form.getByLabelText('Comment'), 'x')
	await userEvent.click(addButton())

	await waitFor(() => expect(form.getByLabelText('Date')).toHaveFocus())
})

test('an add answered while a removal is confirmed leaves focus on Keep', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall] })
	const adding = hold('AddContactFieldEntry', added)

	renderPanel()
	const comment = (await addForm()).getByLabelText('Comment')
	await userEvent.type(comment, 'x')
	await userEvent.click(addButton())
	await waitFor(() => expect(adding.called).toHaveBeenCalledOnce())
	await userEvent.click(screen.getByRole('button', { name: 'Remove entry: Sep 1, 2026' }))
	adding.release()

	await waitFor(() => expect(comment).toHaveValue(''))
	expect(screen.getByRole('button', { name: 'Keep' })).toHaveFocus()
})

test('a refused add shows the reason and keeps the draft', async () => {
	speakTemplates()
	serveCatalogue([history])
	serveValues({ history: null })
	capture('AddContactFieldEntry', refusal('VALIDATION', 'field_entry_empty'))

	const { graph } = renderPanel()
	const form = await addForm()
	await userEvent.type(form.getByLabelText('Comment'), 'x')
	await userEvent.click(addButton())

	expect(await screen.findByRole('alert')).toHaveTextContent('Fill in at least one part of the entry.')
	expect(form.getByLabelText('Comment')).toHaveValue('x')
	expect(graph.refetch).not.toHaveBeenCalled()
})

test('a full list names its most entries and reads the list again', async () => {
	speakTemplates()
	serveCatalogue([history])
	serveValues({ history: null })
	capture('AddContactFieldEntry', refusal('CONFLICT', 'field_entries_full', { max: 500 }))

	const { graph } = renderPanel()
	await userEvent.type((await addForm()).getByLabelText('Comment'), 'x')
	await userEvent.click(addButton())

	expect(await screen.findByRole('alert')).toHaveTextContent('This list is full. It holds 500 entries at most.')
	expect(graph.refetch).toHaveBeenCalledWith(['ContactFieldValues'])
})

test('an add to a contact that is gone says so', async () => {
	speakTemplates()
	serveCatalogue([history])
	serveValues({ history: null })
	capture('AddContactFieldEntry', refusal('NOT_FOUND', 'contact_not_found'))

	renderPanel()
	await userEvent.type((await addForm()).getByLabelText('Comment'), 'x')
	await userEvent.click(addButton())

	expect(await screen.findByRole('alert')).toHaveTextContent('That contact no longer exists.')
})

test('an add to a repeater archived elsewhere reads the catalogue again', async () => {
	speakTemplates()
	serveCatalogue([history, jobTitle])
	serveValues({ history: null, jobTitle: null })
	capture('AddContactFieldEntry', refusal('VALIDATION', 'field_unknown'))

	const { graph } = renderPanel()
	const form = await addForm()
	serveCatalogue([jobTitle])
	await userEvent.type(form.getByLabelText('Comment'), 'x')
	await userEvent.click(addButton())

	await waitFor(() => expect(screen.queryByRole('group', { name: 'History' })).not.toBeInTheDocument())
	expect(graph.refetch).toHaveBeenCalledWith(['Fields'])
	expect(screen.getByRole('alert')).toHaveTextContent('That field is not one this contact holds.')
	expect(screen.getByRole('heading', { name: 'Fields' })).toHaveFocus()
})

test('an add that fails otherwise shows the fallback', async () => {
	serveCatalogue([history])
	serveValues({ history: null })
	capture('AddContactFieldEntry', { errors: [{ message: 'boom' }] })

	renderPanel()
	await userEvent.type((await addForm()).getByLabelText('Comment'), 'x')
	await userEvent.click(addButton())

	expect(await screen.findByRole('alert')).toHaveTextContent('The entry could not be added.')
})

test('Enter in the add form adds the entry and never sends the field values', async () => {
	serveCatalogue([history, jobTitle])
	serveValues({ history: null, jobTitle: null })
	const adding = capture('AddContactFieldEntry', added)
	const written = captureWrite()

	renderPanel()
	const form = await addForm()
	await userEvent.type(form.getByLabelText('Comment'), 'x')
	await userEvent.type(form.getByLabelText('Date'), '{Enter}')

	await waitFor(() => expect(adding).toHaveBeenCalledOnce())
	expect(written).not.toHaveBeenCalled()
})

test('Enter in an untouched add form sends nothing', async () => {
	serveCatalogue([history])
	serveValues({ history: null })
	const adding = capture('AddContactFieldEntry', added)

	renderPanel()
	await userEvent.type((await addForm()).getByLabelText('Date'), '{Enter}')

	await act(async () => {})
	expect(adding).not.toHaveBeenCalled()
})

test('Remove asks first and puts focus on Keep', async () => {
	serveCatalogue([history])
	serveValues({ history: [offerSent, firstCall] })
	const removing = capture('DeleteContactFieldEntry', removed)

	renderPanel()
	await userEvent.click(await screen.findByRole('button', { name: 'Remove entry: Sep 1, 2026' }))

	const item = within(screen.getByRole('listitem', { name: 'Sep 1, 2026' }))
	expect(item.getByRole('group', { name: 'Remove this entry?' })).toBeInTheDocument()
	expect(item.getByRole('button', { name: 'Remove' })).toBeInTheDocument()
	expect(item.getByRole('button', { name: 'Keep' })).toHaveFocus()
	expect(removing).not.toHaveBeenCalled()
})

test('Keep leaves the entry and puts focus back on Remove entry', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall] })
	const removing = capture('DeleteContactFieldEntry', removed)

	renderPanel()
	await userEvent.click(await screen.findByRole('button', { name: 'Remove entry: Sep 1, 2026' }))
	await userEvent.click(screen.getByRole('button', { name: 'Keep' }))

	expect(screen.queryByRole('group', { name: 'Remove this entry?' })).not.toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Remove entry: Sep 1, 2026' })).toHaveFocus()
	expect(removing).not.toHaveBeenCalled()
})

test('confirming a removal sends the entry id and reads the list again', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall] })
	const removing = capture('DeleteContactFieldEntry', removed)

	const { graph } = renderPanel()
	await removeEntry('Sep 1, 2026')

	await waitFor(() =>
		expect(removing).toHaveBeenCalledWith({ contactId: contactID, field: 'history', entryId: ID1 }),
	)
	await waitFor(() => expect(graph.refetch).toHaveBeenCalledWith(['ContactFieldValues']))
})

test('a removal in flight disables its own row and every other row', async () => {
	serveCatalogue([history])
	serveValues({ history: [offerSent, firstCall] })
	const removing = hold('DeleteContactFieldEntry', removed)

	renderPanel()
	await removeEntry('Sep 1, 2026')
	await waitFor(() => expect(removing.called).toHaveBeenCalledOnce())

	expect(screen.getByRole('button', { name: 'Remove' })).toHaveAttribute('aria-disabled', 'true')
	expect(screen.getByRole('button', { name: 'Keep' })).toHaveAttribute('aria-disabled', 'true')
	expect(screen.getByRole('button', { name: 'Remove entry: Sep 10, 2026' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
	removing.release()
})

test('a removed entry stays disabled until the graph stops answering it', async () => {
	serveCatalogue([history])
	const change = serveChangingValues({ history: [offerSent, firstCall] })
	capture('DeleteContactFieldEntry', removed)

	const { graph } = renderPanel()
	await removeEntry('Sep 1, 2026')
	await waitFor(() => expect(graph.refetch).toHaveBeenCalled())

	const settling = await screen.findByRole('button', { name: 'Remove entry: Sep 1, 2026' })
	expect(settling).toHaveAttribute('aria-disabled', 'true')
	change({ history: [offerSent] })
	act(() => graph.refetch(['ContactFieldValues']))
	await waitFor(() => expect(screen.queryByRole('listitem', { name: 'Sep 1, 2026' })).not.toBeInTheDocument())
})

test('a removal moves focus to the next entry', async () => {
	serveCatalogue([history])
	serveValues({ history: [offerSent, firstCall] })
	capture('DeleteContactFieldEntry', removed)

	renderPanel()
	await removeEntry('Sep 10, 2026')

	await waitFor(() => expect(firstButton(screen.getByRole('listitem', { name: 'Sep 1, 2026' }))).toHaveFocus())
})

test('removing the oldest entry moves focus to the one above it', async () => {
	serveCatalogue([history])
	serveValues({ history: [offerSent, firstCall] })
	capture('DeleteContactFieldEntry', removed)

	renderPanel()
	await removeEntry('Sep 1, 2026')

	await waitFor(() => expect(firstButton(screen.getByRole('listitem', { name: 'Sep 10, 2026' }))).toHaveFocus())
})

test('a removal passes over an entry that is still leaving', async () => {
	serveCatalogue([history])
	serveValues({ history: [offerSent, firstCall] })
	capture('DeleteContactFieldEntry', removed)

	renderPanel()
	await removeEntry('Sep 1, 2026')
	await waitFor(() => expect(firstButton(screen.getByRole('listitem', { name: 'Sep 10, 2026' }))).toHaveFocus())
	await removeEntry('Sep 10, 2026')

	await waitFor(async () => expect((await addForm()).getByLabelText('Date')).toHaveFocus())
})

test('a removal answered while typing in the add form leaves focus there', async () => {
	serveCatalogue([history])
	serveValues({ history: [offerSent, firstCall] })
	const removing = hold('DeleteContactFieldEntry', removed)

	renderPanel()
	await removeEntry('Sep 1, 2026')
	await waitFor(() => expect(removing.called).toHaveBeenCalledOnce())
	const comment = (await addForm()).getByLabelText('Comment')
	await userEvent.type(comment, 'x')
	removing.release()

	await waitFor(() => expect(screen.getByRole('button', { name: 'Remove entry: Sep 1, 2026' })).toBeInTheDocument())
	expect(comment).toHaveFocus()
})

test('removing the last entry moves focus to the add form', async () => {
	serveCatalogue([history])
	const change = serveChangingValues({ history: [firstCall] })
	capture('DeleteContactFieldEntry', removed)

	renderPanel()
	await screen.findByRole('listitem', { name: 'Sep 1, 2026' })
	change({ history: null })
	await removeEntry('Sep 1, 2026')

	expect(await screen.findByText('No entries yet.')).toBeInTheDocument()
	await waitFor(async () => expect((await addForm()).getByLabelText('Date')).toHaveFocus())
})

test('a removal answered as not found says so and reads the list again', async () => {
	speakTemplates()
	serveCatalogue([history])
	serveValues({ history: [firstCall] })
	capture('DeleteContactFieldEntry', refusal('NOT_FOUND', 'field_entry_not_found'))

	const { graph } = renderPanel()
	await removeEntry('Sep 1, 2026')

	expect(await screen.findByRole('alert')).toHaveTextContent('That entry no longer exists.')
	expect(graph.refetch).toHaveBeenCalledWith(['ContactFieldValues'])
	expect(screen.getByRole('button', { name: 'Remove entry: Sep 1, 2026' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('a refused removal keeps the entry usable', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall] })
	capture('DeleteContactFieldEntry', { errors: [{ message: 'boom' }] })

	renderPanel()
	await removeEntry('Sep 1, 2026')

	expect(await screen.findByRole('alert')).toHaveTextContent('The entry could not be removed.')
	const kept = screen.getByRole('button', { name: 'Remove entry: Sep 1, 2026' })
	expect(kept).not.toHaveAttribute('aria-disabled', 'true')
	await waitFor(() => expect(kept).toHaveFocus())
})

test('a second removal clears the last failure while it runs', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall] })
	capture('DeleteContactFieldEntry', { errors: [{ message: 'boom' }] })

	renderPanel()
	await removeEntry('Sep 1, 2026')
	await screen.findByRole('alert')
	const removing = hold('DeleteContactFieldEntry', { errors: [{ message: 'boom' }] })
	await removeEntry('Sep 1, 2026')
	await waitFor(() => expect(removing.called).toHaveBeenCalledOnce())

	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	removing.release()
	expect(await screen.findByRole('alert')).toHaveTextContent('The entry could not be removed.')
})

test('a removal from a repeater archived elsewhere reads the catalogue again', async () => {
	speakTemplates()
	serveCatalogue([history, jobTitle])
	serveValues({ history: [firstCall], jobTitle: null })
	capture('DeleteContactFieldEntry', refusal('VALIDATION', 'field_unknown'))

	const { graph } = renderPanel()
	await screen.findByRole('listitem', { name: 'Sep 1, 2026' })
	serveCatalogue([jobTitle])
	await removeEntry('Sep 1, 2026')

	await waitFor(() => expect(screen.queryByRole('group', { name: 'History' })).not.toBeInTheDocument())
	expect(graph.refetch).toHaveBeenCalledWith(['Fields'])
	expect(screen.getByRole('alert')).toHaveTextContent('That field is not one this contact holds.')
	expect(screen.getByRole('heading', { name: 'Fields' })).toHaveFocus()
})

test('saving fields never sends a repeater', async () => {
	serveCatalogue([history, jobTitle])
	serveValues({ history: [firstCall], jobTitle: null })
	const written = captureWrite()

	renderPanel()
	await userEvent.type(await screen.findByLabelText('Job title'), 'Rear Admiral')
	const save = screen.getByRole('button', { name: 'Save fields' })
	await userEvent.click(save)

	await waitFor(() =>
		expect(written).toHaveBeenCalledWith({ contactId: contactID, values: { jobTitle: 'Rear Admiral' } }),
	)
	expect(save.closest('form')).not.toContainElement(screen.getByRole('group', { name: 'History' }))
})

test('a contact holding only repeaters shows no Save fields', async () => {
	serveCatalogue([history])
	serveValues({ history: [firstCall] })

	renderPanel()
	await screen.findByRole('group', { name: 'History' })

	expect(screen.queryByRole('button', { name: 'Save fields' })).not.toBeInTheDocument()
})

test('a draft stays with the contact it was typed on', async () => {
	serveCatalogue([history])
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

	const { showContact } = renderPanel()
	await userEvent.type((await addForm()).getByLabelText('Comment'), 'Typed on the first contact.')
	showContact(otherContactID)

	await screen.findByRole('listitem', { name: 'Sep 10, 2026' })
	const form = await addForm()
	expect(form.getByLabelText('Comment')).toHaveValue('')
	expect(form.getByLabelText('Date')).toHaveValue(today)
})

test('entries wait for the stored values before they show', async () => {
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
	expect(await screen.findByRole('listitem', { name: 'Sep 1, 2026' })).toBeInTheDocument()
})

test('an entry holding only a second date is named by that detail line', async () => {
	serveCatalogue([visits])
	serveValues({ visits: [{ id: ID2, followUpOn: '2026-10-01' }] })

	renderPanel()

	expect(await itemNames('Visits')).toEqual(['Follow up on: Oct 1, 2026'])
})

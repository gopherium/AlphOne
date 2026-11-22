// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server, textClasses } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { resetLocaleData, setLocaleData } from '@wordpress/i18n'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { configureAppErrorText } from '../i18n/errors'
import { createAppRouter } from '../router'
import {
	anaID,
	brunoID,
	detailFor,
	expectFieldIn,
	formRowChildren,
	identityID1,
	identityID2,
	serveAnaGarcia,
} from './contactScreen'
import { busyClasses, compactClasses, renderAt, renderRoutedAt } from './render'

/**
 * Waits for the question the page asks before it leaves an unsaved name.
 * @returns The question.
 */
function discardQuestion() {
	return screen.findByRole('alertdialog', { name: 'Discard changes?' })
}

/**
 * Serves the contacts list empty, so a test can land on it.
 */
function serveEmptyList() {
	server.use(
		graphql.query('ContactPage', () =>
			HttpResponse.json({ data: { contactPage: { __typename: 'ContactPage', items: [], total: 0, limit: 20 } } }),
		),
	)
}

/**
 * Serves a contact named Ana García whose name a rename changes.
 * @returns The way to rename the contact to Maria Perez on the server.
 */
function serveRenames() {
	let currentName = 'Ana García'
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({ data: { contact: detailFor(String(variables.id), currentName, []) } }),
		),
		graphql.mutation('RenameContact', ({ variables }) => {
			currentName = String(variables.name)
			return HttpResponse.json({
				data: { renameContact: { __typename: 'Contact', id: String(variables.id), name: currentName } },
			})
		}),
	)
	return () => {
		currentName = 'Maria Perez'
	}
}

/**
 * Returns the link of the page's breadcrumbs that leads to the contacts.
 * @returns The link.
 */
function contactsCrumb() {
	return within(screen.getByRole('navigation', { name: 'Breadcrumbs' })).getByRole('link', { name: 'Contacts' })
}

/** The answer of a rename the server refuses. */
const refusedRename = {
	data: null,
	errors: [{ message: 'invalid contact details', extensions: { code: 'VALIDATION' } }],
}

/**
 * Opens the contact, edits its name and presses Save.
 * @returns The name field.
 */
async function saveEditedName() {
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')
	await userEvent.type(name, ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))
	return name
}

afterEach(() => {
	resetLocaleData(undefined, 'alphone-whatsapp')
})

beforeEach(serveAnaGarcia)

test('loads the contact page in a chunk of its own, off the first paint', () => {
	const { component } = createAppRouter().routesById['/contacts/$contactId'].options

	expect(typeof (component as { preload?: unknown }).preload).toBe('function')
})

test('creates a contact, confirms it and opens its detail', async () => {
	const newID = '0198c000-0000-7000-8000-000000000009'
	server.use(
		graphql.mutation('CreateContact', () =>
			HttpResponse.json({
				data: { createContact: { __typename: 'Contact', id: newID, name: 'New Ltd' } },
			}),
		),
		graphql.query('ContactDetail', () =>
			HttpResponse.json({ data: { contact: detailFor(newID, 'New Ltd', []) } }),
		),
	)
	renderAt('/contacts/new')
	const create = await screen.findByRole('button', { name: 'Create contact' })
	expect(create).toHaveAttribute('aria-disabled', 'true')

	await userEvent.type(screen.getByLabelText('Name'), 'New Ltd')
	await userEvent.click(create)

	expect(
		await screen.findByRole('heading', { name: 'New Ltd' }),
	).toBeInTheDocument()
	expect(screen.getByText('No identities yet.')).toBeInTheDocument()
	expect(screen.getByText('Contact added.')).toBeInTheDocument()
})

test('reports invalid contact details on create', async () => {
	server.use(
		graphql.mutation('CreateContact', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'name is required', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt('/contacts/new')
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))

	await screen.findByText('name is required')
	expect(name).toHaveAccessibleDescription('name is required')
	expect(name).toHaveFocus()
	expect(document.querySelector('.godmin-toast')).toBeNull()

	await userEvent.type(name, 'Y')

	await waitFor(() => expect(screen.queryByText('name is required')).not.toBeInTheDocument())
})

test('says in a toast that the contact could not be created when the failure is about no field', async () => {
	server.use(
		graphql.mutation('CreateContact', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/contacts/new')
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))

	expect(await screen.findByText('Could not create the contact.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(screen.queryByText('Contact added.')).not.toBeInTheDocument()
	expect(name).toHaveValue('X')
	await waitFor(() =>
		expect(screen.getByRole('button', { name: 'Create contact' })).not.toHaveAttribute('aria-disabled', 'true'),
	)
})

test('drops the session when the create is unauthorized', async () => {
	server.use(
		graphql.mutation('CreateContact', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt('/contacts/new')

	await userEvent.type(await screen.findByLabelText('Name'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})

test('lays the name and Create contact on one form row at the readable width', async () => {
	renderAt('/contacts/new')
	await screen.findByLabelText('Name')

	const create = screen.getByRole('button', { name: 'Create contact' })
	const cells = formRowChildren('Create contact')
	expect(cells).toHaveLength(2)
	expectFieldIn(cells[0], 'Name')
	expect(cells[1]).toBe(create)
	expect(create.closest('form')).toHaveClass('godmin-form')
	expect(create.closest('form')).not.toHaveClass('godmin-form--inline')
})

test('shows the contact detail with its identities', async () => {
	renderAt(`/contacts/${anaID}`)

	expect(
		await screen.findByRole('heading', { name: 'Ana García' }),
	).toBeInTheDocument()
	expect(screen.getByText('WhatsApp: 184467235 (Ana G)')).toBeInTheDocument()
	expect(screen.getByText('WhatsApp: 184467236')).toBeInTheDocument()
	expect(screen.getByText('Created 06/07/2026')).toBeInTheDocument()
})

test('names each identity channel by its name instead of its key', async () => {
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: {
					contact: detailFor(String(variables.id), 'Maria Perez', [
						{ id: identityID1, channel: 'email', identifier: 'maria.perez@example.com', display_name: 'Maria Perez' },
						{ id: identityID2, channel: 'phone', identifier: '184467235', display_name: '' },
					]),
				},
			}),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Maria Perez' })

	expect(screen.getByText('Email: maria.perez@example.com (Maria Perez)')).toBeInTheDocument()
	expect(screen.getByText('Phone: 184467235')).toBeInTheDocument()
	expect(screen.queryByText(/^(email|phone):/)).not.toBeInTheDocument()
})

test('names a plugin channel in the language the page shows', async () => {
	setLocaleData({ 'contact channel\u0004WhatsApp': ['Chat app'] }, 'alphone-whatsapp')
	renderAt(`/contacts/${anaID}`)

	expect(await screen.findByText('Chat app: 184467236')).toBeInTheDocument()
})

test('shows a channel that no one names by its key', async () => {
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: {
					contact: detailFor(String(variables.id), 'Maria Perez', [
						{ id: identityID1, channel: 'telegram', identifier: '184467235', display_name: '' },
					]),
				},
			}),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Maria Perez' })

	expect(screen.getByText('telegram: 184467235')).toBeInTheDocument()
})

test('sets the name, the identities and the creation date in the details and the tasks in the main work', async () => {
	renderAt(`/contacts/${anaID}`)

	const created = await screen.findByText('Created 06/07/2026')
	const details = screen.getByRole('region', { name: 'Contact details' })
	expect(details).toHaveClass('godmin-page__aside')
	expect(details).toContainElement(screen.getByLabelText('Name'))
	expect(details).toContainElement(screen.getByRole('heading', { name: 'Identities' }))
	expect(details).toContainElement(screen.getByRole('list', { name: 'Identities' }))
	expect(details).toContainElement(screen.getByRole('button', { name: 'Add identity' }))
	expect(details.lastElementChild).toBe(created)
	expect(screen.getByRole('heading', { name: 'Tasks' }).closest('.godmin-page__main')).not.toBeNull()
})

test('sets the plugin panels in the main work under the tasks', async () => {
	server.use(
		graphql.query('Fields', () =>
			HttpResponse.json({
				data: {
					fields: [{
						__typename: 'FieldDefinition',
						id: '0198c000-0000-7000-8000-000000000021',
						name: 'region',
						label: 'Region',
						kind: 'TEXT',
						subFields: [],
					}],
				},
			}),
		),
		graphql.query('ContactFieldValues', ({ variables }) =>
			HttpResponse.json({ data: { contact: { __typename: 'Contact', id: variables.id, region: null } } }),
		),
	)
	renderAt(`/contacts/${anaID}`)

	const fields = await screen.findByRole('heading', { name: 'Fields' })
	expect(fields.closest('.godmin-page__main')).not.toBeNull()
	const tasks = screen.getByRole('heading', { name: 'Tasks' })
	expect(tasks.compareDocumentPosition(fields) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('sets the section headings a size above the field labels', async () => {
	const large = textClasses('heading-lg')
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { level: 2, name: 'Ana García' })

	const identities = screen.getByRole('heading', { level: 3, name: 'Identities' })
	const tasks = screen.getByRole('heading', { level: 3, name: 'Tasks' })
	expect([...identities.classList]).toEqual(expect.arrayContaining(large))
	expect([...tasks.classList]).toEqual(expect.arrayContaining(large))
})

test('renames a contact', async () => {
	let currentName = 'Ana García'
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: { contact: detailFor(String(variables.id), currentName, []) },
			}),
		),
		graphql.mutation('RenameContact', ({ variables }) => {
			currentName = String(variables.name)
			return HttpResponse.json({
				data: {
					renameContact: {
						__typename: 'Contact',
						id: String(variables.id),
						name: currentName,
					},
				},
			})
		}),
	)
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')
	await userEvent.clear(name)
	expect(screen.getByRole('button', { name: 'Save' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)

	await userEvent.type(name, 'Ana García Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByText('Name updated.')).toBeInTheDocument()
	expect(screen.getByRole('heading', { name: 'Ana García Ltd' })).toBeInTheDocument()
	expect(name).toHaveValue('Ana García Ltd')
})

test('saves the name when the reader presses Enter in its field', async () => {
	serveRenames()
	renderAt(`/contacts/${anaID}`)

	await userEvent.type(await screen.findByLabelText('Name'), ' Ltd{Enter}')

	expect(await screen.findByText('Name updated.')).toBeInTheDocument()
	expect(screen.getByRole('heading', { name: 'Ana García Ltd' })).toBeInTheDocument()
})

test('sets Save last in the page head, compact and off while the name is clean', async () => {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	const save = screen.getByRole('button', { name: 'Save' })
	const head = save.closest('.godmin-page__head') as HTMLElement
	expect(head).not.toBeNull()
	expect(within(head).getAllByRole('button').at(-1)).toBe(save)
	expect([...save.classList]).toEqual(expect.arrayContaining(compactClasses()))
	expect(save).toHaveAttribute('aria-disabled', 'true')
})

test('turns Save on after an edit and off again when the name reads as saved', async () => {
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')
	const save = screen.getByRole('button', { name: 'Save' })

	await userEvent.type(name, ' Ltd')
	expect(save).not.toHaveAttribute('aria-disabled', 'true')

	await userEvent.type(name, '{Backspace}{Backspace}{Backspace}{Backspace}')
	expect(save).toHaveAttribute('aria-disabled', 'true')
})

test('shows Save loading and locks the name field while the rename runs', async () => {
	server.use(graphql.mutation('RenameContact', () => new Promise(() => {})))
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')
	const save = screen.getByRole('button', { name: 'Save' })

	await userEvent.type(name, ' Ltd')
	await userEvent.click(save)

	await waitFor(() => expect(name).toHaveAttribute('readonly'))
	expect(name).toHaveValue('Ana García Ltd')
	expect(save).toHaveAttribute('aria-disabled', 'true')
	expect([...save.classList]).toEqual(expect.arrayContaining(busyClasses()))
})

test('keeps the focus in the name field while Enter saves it', async () => {
	server.use(graphql.mutation('RenameContact', () => new Promise(() => {})))
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, ' Ltd{Enter}')

	await waitFor(() => expect(name).toHaveAttribute('readonly'))
	expect(name).toBeEnabled()
	expect(name).toHaveFocus()
})

test('sets the name field alone in a stacked form', async () => {
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')

	const form = name.closest('form')
	expect(form).toHaveClass('godmin-form')
	expect(form).not.toHaveClass('godmin-form--inline')
	expect(name.closest('.godmin-form__row')).toBeNull()
	expect(form).not.toContainElement(screen.getByRole('button', { name: 'Save' }))
})

test('follows the name field in the title and falls back on Contact while it is empty', async () => {
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, ' Ltd')
	expect(screen.getByRole('heading', { level: 2, name: 'Ana García Ltd' })).toBeInTheDocument()

	await userEvent.clear(name)
	expect(screen.getByRole('heading', { level: 2, name: 'Contact' })).toBeInTheDocument()
})

test('stays on the contact with the typed name when the reader keeps editing', async () => {
	serveEmptyList()
	const { router } = renderRoutedAt(`/contacts/${anaID}`)
	await userEvent.type(await screen.findByLabelText('Name'), ' Ltd')

	await userEvent.click(contactsCrumb())
	const question = await discardQuestion()
	expect(question).toHaveAccessibleDescription('The name is not saved.')
	expect(within(question).getByRole('button', { name: 'Discard' }).className).toMatch(/irreversible/)
	await userEvent.click(within(question).getByRole('button', { name: 'Keep editing' }))

	await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
	expect(router.state.location.pathname).toBe(`/contacts/${anaID}`)
	expect(screen.getByLabelText('Name')).toHaveValue('Ana García Ltd')
})

test('stays on the contact with the typed name when the reader presses Escape on the question', async () => {
	serveEmptyList()
	const { router } = renderRoutedAt(`/contacts/${anaID}`)
	await userEvent.type(await screen.findByLabelText('Name'), ' Ltd')

	await userEvent.click(contactsCrumb())
	await discardQuestion()
	await userEvent.keyboard('{Escape}')

	await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
	expect(router.state.location.pathname).toBe(`/contacts/${anaID}`)
	expect(screen.getByLabelText('Name')).toHaveValue('Ana García Ltd')
})

test('leaves the contact when the reader discards an unsaved name', async () => {
	serveEmptyList()
	const { router } = renderRoutedAt(`/contacts/${anaID}`)
	await userEvent.type(await screen.findByLabelText('Name'), ' Ltd')

	await userEvent.click(contactsCrumb())
	await userEvent.click(within(await discardQuestion()).getByRole('button', { name: 'Discard' }))

	await waitFor(() => expect(router.state.location.pathname).toBe('/contacts'))
})

test('asks before the sidebar leaves an unsaved name', async () => {
	const { router } = renderRoutedAt(`/contacts/${anaID}`)
	await userEvent.type(await screen.findByLabelText('Name'), ' Ltd')

	await userEvent.click(within(screen.getByRole('region', { name: 'Navigation' })).getByRole('link', { name: 'Tasks' }))

	expect(await discardQuestion()).toBeInTheDocument()
	expect(router.state.location.pathname).toBe(`/contacts/${anaID}`)
})

test('never asks before leaving a clean contact', async () => {
	serveEmptyList()
	const { router } = renderRoutedAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(contactsCrumb())

	await waitFor(() => expect(router.state.location.pathname).toBe('/contacts'))
	expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
})

test('never asks before leaving a saved name', async () => {
	serveEmptyList()
	serveRenames()
	const { router } = renderRoutedAt(`/contacts/${anaID}`)
	await userEvent.type(await screen.findByLabelText('Name'), ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))
	await screen.findByText('Name updated.')

	await userEvent.click(contactsCrumb())

	await waitFor(() => expect(router.state.location.pathname).toBe('/contacts'))
	expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
})

test('drops an unsaved name when the reader discards it to move on to another contact', async () => {
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: {
					contact: detailFor(String(variables.id), variables.id === anaID ? 'Ana García' : 'Maria Perez', []),
				},
			}),
		),
	)
	const { router } = renderRoutedAt(`/contacts/${anaID}`)
	await userEvent.type(await screen.findByLabelText('Name'), ' Ltd')

	void router.navigate({ to: '/contacts/$contactId', params: { contactId: brunoID } })
	await userEvent.click(within(await discardQuestion()).getByRole('button', { name: 'Discard' }))

	expect(await screen.findByRole('heading', { level: 2, name: 'Maria Perez' })).toBeInTheDocument()
	expect(screen.getByLabelText('Name')).toHaveValue('Maria Perez')
})

/** Adds an identity, which reads the contact again, and expects the page to show the newer name with Save off. */
async function expectNewerNameAfterReread() {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({
				data: {
					addContactIdentity: {
						__typename: 'ContactIdentity',
						id: identityID1,
						channel: 'email',
						identifier: 'maria@example.com',
						displayName: '',
					},
				},
			}),
		),
	)
	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByRole('heading', { level: 2, name: 'Maria Perez' })).toBeInTheDocument()
	expect(screen.getByLabelText('Name')).toHaveValue('Maria Perez')
	expect(screen.getByRole('button', { name: 'Save' })).toHaveAttribute('aria-disabled', 'true')
}

test('follows a newer name from the server once the rename is saved', async () => {
	const renameElsewhere = serveRenames()
	renderAt(`/contacts/${anaID}`)
	await userEvent.type(await screen.findByLabelText('Name'), ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))
	await screen.findByText('Name updated.')
	renameElsewhere()

	await expectNewerNameAfterReread()
})

test('follows a newer name from the server once the reader undid an edit', async () => {
	const renameElsewhere = serveRenames()
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')
	await userEvent.type(name, ' Ltd')
	await userEvent.type(name, '{Backspace}{Backspace}{Backspace}{Backspace}')
	renameElsewhere()

	await expectNewerNameAfterReread()
})

test('reports invalid contact details on rename', async () => {
	server.use(graphql.mutation('RenameContact', () => HttpResponse.json(refusedRename)))

	const name = await saveEditedName()

	await screen.findByText('invalid contact details')
	expect(name).toHaveAccessibleDescription('invalid contact details')
	expect(name).toHaveFocus()
	expect(document.querySelector('.godmin-toast')).toBeNull()
})

test('takes the message about a name away as the reader edits it', async () => {
	server.use(graphql.mutation('RenameContact', () => HttpResponse.json(refusedRename)))
	const name = await saveEditedName()
	await screen.findByText('invalid contact details')

	await userEvent.type(name, ' Inc')

	await waitFor(() => expect(screen.queryByText('invalid contact details')).not.toBeInTheDocument())
	expect(name).toBeValid()
})

test('shows nothing when a refused name is typed again after another one saved', async () => {
	server.use(
		graphql.mutation('RenameContact', ({ variables }) => {
			if (variables.name === 'Ana García Ltd') {
				return HttpResponse.json(refusedRename)
			}
			return HttpResponse.json({
				data: { renameContact: { __typename: 'Contact', id: anaID, name: String(variables.name) } },
			})
		}),
	)
	const name = await saveEditedName()
	await screen.findByText('invalid contact details')

	await userEvent.type(name, ' Inc')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))
	await screen.findByText('Name updated.', { selector: '.godmin-toast' })
	await userEvent.clear(name)
	await userEvent.type(name, 'Ana García Ltd')

	expect(name).toBeValid()
	expect(screen.queryByText('invalid contact details')).not.toBeInTheDocument()
})

test('says in a toast that the contact could not be saved when the failure is about no field', async () => {
	let renames = 0
	server.use(
		graphql.mutation('RenameContact', () => {
			renames += 1
			return HttpResponse.json({ data: null, errors: [{ message: 'bad gateway' }] })
		}),
	)
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByText('Could not save the contact.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(screen.queryByText('Name updated.')).not.toBeInTheDocument()
	expect(name).toHaveValue('Ana García Ltd')
	expect(name).toBeEnabled()
	expect(screen.getByRole('button', { name: 'Save' })).not.toHaveAttribute('aria-disabled', 'true')

	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() => expect(renames).toBe(2))
})

test('drops the session when the rename is unauthorized', async () => {
	server.use(
		graphql.mutation('RenameContact', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})

test('reports when the contact cannot be loaded', async () => {
	server.use(
		graphql.query('ContactDetail', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	renderAt(`/contacts/${anaID}`)

	expect(await screen.findByRole('alert')).toHaveTextContent(/could not be loaded/i)
	expect(screen.getByRole('heading', { level: 2, name: 'Contact' })).toBeInTheDocument()
	expect(contactsCrumb()).toHaveAttribute('href', '/contacts')
	expect(screen.queryByRole('region', { name: 'Contact details' })).not.toBeInTheDocument()
})

test('keeps the breadcrumb and names the kind of record while the contact loads', async () => {
	server.use(graphql.query('ContactDetail', () => new Promise(() => {})))

	renderAt(`/contacts/${anaID}`)

	expect(await screen.findByRole('heading', { level: 2, name: 'Contact' })).toBeInTheDocument()
	expect(contactsCrumb()).toHaveAttribute('href', '/contacts')
	expect(screen.queryByRole('region', { name: 'Contact details' })).not.toBeInTheDocument()
})

test('drops the session when the contact detail is unauthorized', async () => {
	server.use(
		graphql.query('ContactDetail', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)

	const client = renderAt(`/contacts/${anaID}`)

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})



test('shows the reader the sentence the reason names, not the server prose', async () => {
	configureAppErrorText()
	server.use(
		graphql.mutation('CreateContact', () =>
			HttpResponse.json({
				data: null,
				errors: [{
					message: 'contact: empty name',
					extensions: { code: 'VALIDATION', reason: 'contact_name_required' },
				}],
			})),
	)
	renderAt('/contacts/new')

	await userEvent.type(await screen.findByLabelText('Name'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))

	expect(await screen.findByText('A contact needs a name.')).toBeInTheDocument()
	expect(screen.queryByText('contact: empty name')).not.toBeInTheDocument()
})

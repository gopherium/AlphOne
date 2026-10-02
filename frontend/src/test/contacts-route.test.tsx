// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server, textClasses } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { resetLocaleData, setLocaleData } from '@wordpress/i18n'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { configureAppErrorText } from '../i18n/errors'
import { renderAt } from './render'

const anaID = '0198c000-0000-7000-8000-000000000001'
const brunoID = '0198c000-0000-7000-8000-000000000002'
const carlaID = '0198c000-0000-7000-8000-000000000003'
const adaID = '0198c000-0000-7000-8000-000000000004'
const identityID1 = '0198c000-0000-7000-8000-000000000011'
const identityID2 = '0198c000-0000-7000-8000-000000000012'

function contactsPage(named: [string, string][], endCursor: string | null) {
	return {
		__typename: 'ContactConnection',
		edges: named.map(([id, name]) => ({
			__typename: 'ContactEdge',
			node: { __typename: 'Contact', id, name, createdAt: '2026-07-06T10:00:00Z' },
			cursor: id,
		})),
		pageInfo: { __typename: 'PageInfo', hasNextPage: endCursor !== null, endCursor },
	}
}

function pageFor(q: string, after: string | undefined) {
	if (q === 'ada') {
		return contactsPage([[adaID, 'Ada Lovelace']], null)
	}
	if (q !== '') {
		return contactsPage([], null)
	}
	if (after === 'CUR1') {
		return contactsPage([[carlaID, 'Carla']], null)
	}
	return contactsPage([[anaID, 'Ana García'], [brunoID, 'Bruno']], 'CUR1')
}

type IdentityRow = { id: string; channel: string; identifier: string; display_name: string }

function detailFor(id: string, name: string, identities: IdentityRow[]) {
	return {
		__typename: 'Contact',
		id,
		name,
		createdAt: '2026-07-06T10:00:00Z',
		identities: identities.map((identity) => ({
			__typename: 'ContactIdentity',
			id: identity.id,
			channel: identity.channel,
			identifier: identity.identifier,
			displayName: identity.display_name,
		})),
		tasks: {
			__typename: 'TaskConnection',
			edges: [],
			pageInfo: { __typename: 'PageInfo', hasNextPage: false, endCursor: null },
		},
	}
}

/**
 * Returns the direct children of the form row that holds the named button.
 * @param button - The name of the row's submit button.
 * @returns The row's children, in order.
 */
function formRowChildren(button: string) {
	const row = screen.getByRole('button', { name: button }).parentElement
	expect(row).toHaveClass('godmin-form__row')
	expect(row?.parentElement).toHaveClass('godmin-form')
	return [...(row as HTMLElement).children]
}

/**
 * Expects one row child to hold both the labelled control and its label.
 * @param child - The direct child of the form row.
 * @param label - The text of the control's label.
 */
function expectFieldIn(child: Element | undefined, label: string) {
	expect(child).toContainElement(screen.getByLabelText(label))
	expect(child).toContainElement(screen.getByText(label))
}

let listQueries: string[] = []

afterEach(() => {
	resetLocaleData(undefined, 'alphone-whatsapp')
})

beforeEach(() => {
	listQueries = []
	server.use(
		graphql.query('Contacts', ({ variables }) => {
			const q = (variables.q as string | null) ?? ''
			listQueries.push(q)
			return HttpResponse.json({
				data: { contacts: pageFor(q, variables.after as string | undefined) },
			})
		}),
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: {
					contact: detailFor(String(variables.id), 'Ana García', [
						{ id: identityID1, channel: 'whatsapp', identifier: '184467235', display_name: 'Ana G' },
						{ id: identityID2, channel: 'whatsapp', identifier: '184467236', display_name: '' },
					]),
				},
			}),
		),
	)
})

test('ghosts the rows while the contacts arrive', async () => {
	server.use(graphql.query('Contacts', () => new Promise(() => {})))
	renderAt('/contacts')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading contacts…')
	expect(status.closest('.godmin-loading-rows')).not.toBeNull()
})

test('fades the contact table in when it replaces the ghost', async () => {
	renderAt('/contacts')

	const region = await screen.findByRole('region', { name: 'Contacts' })
	expect([...region.classList]).toContain('godmin-arrival')
})

test('serves the contacts screen at /contacts', async () => {
	renderAt('/contacts')

	expect(
		await screen.findByRole('heading', { name: 'Contacts' }),
	).toBeInTheDocument()
	const ana = await screen.findByRole('row', { name: /Ana García/ })
	expect(within(ana).getByText('Jul 6, 2026')).toBeInTheDocument()
	expect(screen.getByRole('row', { name: /Bruno/ })).toBeInTheDocument()
})

test('navigates to the contacts screen from the main menu', async () => {
	renderAt('/')

	await userEvent.click(await screen.findByRole('link', { name: 'Contacts' }))

	expect(
		await screen.findByRole('heading', { name: 'Contacts' }),
	).toBeInTheDocument()
})

test('loads more contacts through the cursor', async () => {
	renderAt('/contacts')
	await screen.findByRole('row', { name: /Ana García/ })

	await userEvent.click(screen.getByRole('button', { name: 'Load more' }))

	expect(await screen.findByRole('row', { name: /Carla/ })).toBeInTheDocument()
	await waitFor(() =>
		expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument(),
	)
})

test('searches contacts once the query settles', async () => {
	renderAt('/contacts')
	await screen.findByRole('row', { name: /Ana García/ })

	await userEvent.type(
		screen.getByRole('textbox', { name: /search contacts/i }),
		'ada',
	)

	expect(await screen.findByRole('row', { name: /Ada Lovelace/ })).toBeInTheDocument()
	expect(listQueries).toContain('ada')
	expect(listQueries).not.toContain('a')
	expect(listQueries).not.toContain('ad')
})

test('shows an empty state when the search finds nothing', async () => {
	renderAt('/contacts')
	await screen.findByRole('row', { name: /Ana García/ })

	await userEvent.type(
		screen.getByRole('textbox', { name: /search contacts/i }),
		'zz',
	)

	expect(await screen.findByText('No contacts found.')).toBeInTheDocument()
	expect(
		screen.getByText('Try a different search, or add one with New contact.'),
	).toBeInTheDocument()
})

test('reports when contacts cannot be loaded', async () => {
	server.use(
		graphql.query('Contacts', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	renderAt('/contacts')

	expect(await screen.findByRole('alert')).toHaveTextContent(/could not be loaded/i)
})

test('drops the session when the contacts request is unauthorized', async () => {
	server.use(
		graphql.query('Contacts', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)

	const client = renderAt('/contacts')

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})

test('creates a contact and opens its detail', async () => {
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

	await userEvent.type(await screen.findByLabelText('Name'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))

	expect(await screen.findByText('name is required')).toBeInTheDocument()
})

test('reports a generic message when the create fails otherwise', async () => {
	server.use(
		graphql.mutation('CreateContact', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/contacts/new')

	await userEvent.type(await screen.findByLabelText('Name'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))

	expect(
		await screen.findByText('The contact could not be created.'),
	).toBeInTheDocument()
	expect(screen.queryByText('Contact added.')).not.toBeInTheDocument()
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
	server.use(
		graphql.mutation('CreateContact', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/contacts/new')
	await userEvent.type(await screen.findByLabelText('Name'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))
	const text = await screen.findByText('The contact could not be created.')
	const notice = text.closest('[role="alert"]') as HTMLElement

	const create = screen.getByRole('button', { name: 'Create contact' })
	const cells = formRowChildren('Create contact')
	expect(cells).toHaveLength(2)
	expectFieldIn(cells[0], 'Name')
	expect(cells[1]).toBe(create)
	expect(create.closest('form')).toHaveClass('godmin-form')
	expect(create.closest('form')).not.toHaveClass('godmin-form--inline')
	expect(notice).toBeInTheDocument()
	expect(notice.closest('.godmin-form__row')).toBeNull()
	expect(notice.closest('form')).toContainElement(create)
})

test('shows the contact detail with its identities', async () => {
	renderAt(`/contacts/${anaID}`)

	expect(
		await screen.findByRole('heading', { name: 'Ana García' }),
	).toBeInTheDocument()
	expect(screen.getByText('WhatsApp: 184467235 (Ana G)')).toBeInTheDocument()
	expect(screen.getByText('WhatsApp: 184467236')).toBeInTheDocument()
	expect(screen.getByText('Created Jul 6, 2026')).toBeInTheDocument()
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

test('sets the creation date beside the contact work', async () => {
	renderAt(`/contacts/${anaID}`)

	const created = await screen.findByText('Created Jul 6, 2026')
	expect(created.closest('.godmin-page__aside')).not.toBeNull()
	expect(screen.getByRole('heading', { name: 'Identities' }).closest('.godmin-page__main')).not.toBeNull()
	expect(screen.getByRole('heading', { name: 'Tasks' }).closest('.godmin-page__main')).not.toBeNull()
})

test('sets the section headings a size above the field labels', async () => {
	const large = textClasses('heading-lg')
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { level: 1, name: 'Ana García' })

	const identities = screen.getByRole('heading', { level: 2, name: 'Identities' })
	const tasks = screen.getByRole('heading', { level: 2, name: 'Tasks' })
	expect([...identities.classList]).toEqual(expect.arrayContaining(large))
	expect([...tasks.classList]).toEqual(expect.arrayContaining(large))
})

test('adds an email identity to the contact', async () => {
	const identities = [
		{ id: identityID1, channel: 'whatsapp', identifier: '184467235', display_name: 'Ana G' },
	]
	let posted: Record<string, unknown> | null = null
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: { contact: detailFor(String(variables.id), 'Ana García', identities) },
			}),
		),
		graphql.mutation('AddContactIdentity', ({ variables }) => {
			posted = variables.identity as Record<string, unknown>
			const created = {
				id: identityID2,
				channel: 'email',
				identifier: 'maria@example.com',
				display_name: 'Work',
			}
			identities.push(created)
			return HttpResponse.json({
				data: {
					addContactIdentity: {
						__typename: 'ContactIdentity',
						id: created.id,
						channel: created.channel,
						identifier: created.identifier,
						displayName: created.display_name,
					},
				},
			})
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), ' Maria@Example.COM ')
	await userEvent.type(screen.getByLabelText('Label'), 'Work')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByText('Email: maria@example.com (Work)')).toBeInTheDocument()
	expect(posted).toEqual({
		channel: 'email',
		identifier: ' Maria@Example.COM ',
		displayName: 'Work',
	})
	expect(screen.getByLabelText('Value')).toHaveValue('')
	expect(screen.getByText('Identity added.')).toBeInTheDocument()
})

test('adds a phone identity through the channel select', async () => {
	const identities: IdentityRow[] = []
	let posted: Record<string, unknown> | null = null
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: { contact: detailFor(String(variables.id), 'Ana García', identities) },
			}),
		),
		graphql.mutation('AddContactIdentity', ({ variables }) => {
			posted = variables.identity as Record<string, unknown>
			const created = { id: identityID2, channel: 'phone', identifier: '+184467235', display_name: '' }
			identities.push(created)
			return HttpResponse.json({
				data: {
					addContactIdentity: {
						__typename: 'ContactIdentity',
						id: created.id,
						channel: created.channel,
						identifier: created.identifier,
						displayName: created.display_name,
					},
				},
			})
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByLabelText('Channel'))
	await userEvent.click(await screen.findByRole('option', { name: 'Phone' }))
	await userEvent.type(screen.getByLabelText('Value'), '+184 467 235')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByText('Phone: +184467235')).toBeInTheDocument()
	expect(posted).toMatchObject({ channel: 'phone', identifier: '+184 467 235' })
})

test('marks the channel the reader chose as the selected option', async () => {
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({ data: { contact: detailFor(String(variables.id), 'Ana García', []) } }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByLabelText('Channel'))

	expect(await screen.findByRole('option', { name: 'Email' })).toHaveAttribute(
		'aria-selected',
		'true',
	)
})

test('names the owner when the identity belongs to someone else', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{
					message: 'contact: identity already exists',
					extensions: { code: 'CONFLICT', ownerContactId: brunoID, ownerName: 'Bruno' },
				}],
			}),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('Already on contact Bruno.')
})

test('shows the backend message when the identity is invalid', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'contact: empty identifier', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'abc')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('contact: empty identifier')
})

test('falls back to the backend message when the conflict names no owner', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{
					message: 'contact: identity already exists',
					extensions: { code: 'CONFLICT', ownerContactId: brunoID },
				}],
			}),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'contact: identity already exists',
	)
})



test('reports a generic message when the identity add fails otherwise', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The identity could not be added.',
	)
	expect(screen.queryByText('Identity added.')).not.toBeInTheDocument()
})

test('lays the channel, value, label and Add identity on one form row', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })
	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))
	const notice = await screen.findByRole('alert')

	const add = screen.getByRole('button', { name: 'Add identity' })
	const cells = formRowChildren('Add identity')
	expect(cells).toHaveLength(4)
	expectFieldIn(cells[0], 'Channel')
	expectFieldIn(cells[1], 'Value')
	expectFieldIn(cells[2], 'Label')
	expect(cells[3]).toBe(add)
	expect(notice.closest('.godmin-form__row')).toBeNull()
	expect(notice.closest('form')).toContainElement(add)
})

test('drops the session when the identity add is unauthorized', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('drops the session when the identity removal is unauthorized', async () => {
	server.use(
		graphql.mutation('DeleteContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByRole('button', { name: 'Remove 184467235' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('removes an identity', async () => {
	let identities = [
		{ id: identityID1, channel: 'whatsapp', identifier: '184467235', display_name: 'Ana G' },
		{ id: identityID2, channel: 'email', identifier: 'maria@example.com', display_name: '' },
	]
	let deleted = ''
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: { contact: detailFor(String(variables.id), 'Ana García', identities) },
			}),
		),
		graphql.mutation('DeleteContactIdentity', ({ variables }) => {
			deleted = String(variables.identityId)
			identities = identities.filter((identity) => identity.id !== deleted)
			return HttpResponse.json({ data: { deleteContactIdentity: true } })
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	expect(screen.getByText('Email: maria@example.com')).toBeInTheDocument()

	await userEvent.click(screen.getByRole('button', { name: 'Remove maria@example.com' }))

	await waitFor(() =>
		expect(screen.queryByText('Email: maria@example.com')).not.toBeInTheDocument(),
	)
	expect(deleted).toBe(identityID2)
	expect(screen.getByText('WhatsApp: 184467235 (Ana G)')).toBeInTheDocument()
	expect(screen.getByText('Identity removed.')).toBeInTheDocument()
})

test('spins only the remove button pressed', async () => {
	server.use(graphql.mutation('DeleteContactIdentity', () => new Promise(() => {})))
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByRole('button', { name: 'Remove 184467235' }))

	await waitFor(() =>
		expect(screen.getByRole('button', { name: 'Remove 184467235' })).toHaveAttribute('aria-disabled', 'true'),
	)
	expect(screen.getByRole('button', { name: 'Remove 184467236' })).not.toHaveAttribute('aria-disabled', 'true')
})

test('reports a failed removal', async () => {
	server.use(
		graphql.mutation('DeleteContactIdentity', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByRole('button', { name: 'Remove 184467235' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The identity could not be removed.',
	)
	expect(screen.queryByText('Identity removed.')).not.toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Remove 184467235' })).not.toHaveAttribute('aria-disabled', 'true')
})

test('shows each identity remove as an icon named after its identifier', async () => {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	const remove = screen.getByRole('button', { name: 'Remove 184467235' })
	expect(remove.textContent).toBe('')
	expect(remove.querySelector('svg')).not.toBeNull()
})

test('lists each identity in the bordered log list with its trash at the row end', async () => {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	const list = screen.getByRole('list', { name: 'Identities' })
	expect(list).toHaveClass('godmin-log-list')
	expect(within(list).getAllByRole('listitem')).toHaveLength(2)
	for (const [text, identifier] of [
		['WhatsApp: 184467235 (Ana G)', '184467235'],
		['WhatsApp: 184467236', '184467236'],
	]) {
		const item = within(list).getByRole('listitem', { name: text })
		const line = within(item).getByText(text)
		const remove = within(item).getByRole('button', { name: `Remove ${identifier}` })
		expect(line.closest('.godmin-log-list__label')).not.toBeNull()
		expect(remove.closest('.godmin-log-list__actions')).not.toBeNull()
		expect(line).not.toContainElement(remove)
		expect(line.closest('.godmin-log-list__header')).toContainElement(remove)
	}
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

	expect(
		await screen.findByRole('heading', { name: 'Ana García Ltd' }),
	).toBeInTheDocument()
	expect(screen.getByText('Contact renamed.')).toBeInTheDocument()
})

test('lays the name and Save on one form row', async () => {
	server.use(
		graphql.mutation('RenameContact', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'bad gateway' }] }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await userEvent.type(await screen.findByLabelText('Name'), ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))
	const text = await screen.findByText('The contact could not be renamed.')
	const notice = text.closest('[role="alert"]') as HTMLElement

	const save = screen.getByRole('button', { name: 'Save' })
	const cells = formRowChildren('Save')
	expect(cells).toHaveLength(2)
	expectFieldIn(cells[0], 'Name')
	expect(cells[1]).toBe(save)
	expect(notice).toBeInTheDocument()
	expect(notice.closest('.godmin-form__row')).toBeNull()
	expect(notice.closest('form')).toContainElement(save)
})

test('fills the column with the rename and add identity forms', async () => {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	for (const button of ['Save', 'Add identity']) {
		expect(screen.getByRole('button', { name: button }).closest('form')).toHaveClass(
			'godmin-form',
			'godmin-form--inline',
		)
	}
})

test('reports invalid contact details on rename', async () => {
	server.use(
		graphql.mutation('RenameContact', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'invalid contact details', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByText('invalid contact details')).toBeInTheDocument()
})

test('reports a generic message when the rename fails otherwise', async () => {
	server.use(
		graphql.mutation('RenameContact', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'bad gateway' }] }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(
		await screen.findByText('The contact could not be renamed.'),
	).toBeInTheDocument()
	expect(screen.queryByText('Contact renamed.')).not.toBeInTheDocument()
})

test('surfaces the backend message for unreadable rename rejections', async () => {
	server.use(
		graphql.mutation('RenameContact', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'invalid contact details', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByText('invalid contact details')).toBeInTheDocument()
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

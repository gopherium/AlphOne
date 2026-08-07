import { http, HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { renderAt } from './render'

const anaID = '0198c000-0000-7000-8000-000000000001'
const brunoID = '0198c000-0000-7000-8000-000000000002'
const carlaID = '0198c000-0000-7000-8000-000000000003'
const adaID = '0198c000-0000-7000-8000-000000000004'
const identityID1 = '0198c000-0000-7000-8000-000000000011'
const identityID2 = '0198c000-0000-7000-8000-000000000012'

function contactRow(id: string, name: string) {
	return { id, name, created_at: '2026-07-06T10:00:00Z' }
}

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

let listQueries: string[] = []

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
		http.post('/api/contacts', () =>
			HttpResponse.json(contactRow(newID, 'New Ltd'), { status: 201 }),
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
})

test('reports invalid contact details on create', async () => {
	server.use(
		http.post('/api/contacts', () =>
			HttpResponse.json({ error: 'name is required' }, { status: 422 }),
		),
	)
	renderAt('/contacts/new')

	await userEvent.type(await screen.findByLabelText('Name'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))

	expect(await screen.findByText('name is required')).toBeInTheDocument()
})

test('reports a generic message when the create fails otherwise', async () => {
	server.use(
		http.post('/api/contacts', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt('/contacts/new')

	await userEvent.type(await screen.findByLabelText('Name'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))

	expect(
		await screen.findByText('The contact could not be created.'),
	).toBeInTheDocument()
})

test('drops the session when the create is unauthorized', async () => {
	server.use(
		http.post('/api/contacts', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
	)
	const client = renderAt('/contacts/new')

	await userEvent.type(await screen.findByLabelText('Name'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create contact' }))

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})

test('shows the contact detail with its identities', async () => {
	renderAt(`/contacts/${anaID}`)

	expect(
		await screen.findByRole('heading', { name: 'Ana García' }),
	).toBeInTheDocument()
	expect(screen.getByText('whatsapp: 184467235 (Ana G)')).toBeInTheDocument()
	expect(screen.getByText('whatsapp: 184467236')).toBeInTheDocument()
	expect(screen.getByText('Created Jul 6, 2026')).toBeInTheDocument()
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
		http.post('/api/contacts/:id/identities', async ({ request }) => {
			posted = (await request.json()) as Record<string, unknown>
			const created = {
				id: identityID2,
				channel: 'email',
				identifier: 'maria@example.com',
				display_name: 'Work',
			}
			identities.push(created)
			return HttpResponse.json(created, { status: 201 })
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), ' Maria@Example.COM ')
	await userEvent.type(screen.getByLabelText('Label'), 'Work')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByText('email: maria@example.com (Work)')).toBeInTheDocument()
	expect(posted).toEqual({
		channel: 'email',
		identifier: ' Maria@Example.COM ',
		display_name: 'Work',
	})
	expect(screen.getByLabelText('Value')).toHaveValue('')
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
		http.post('/api/contacts/:id/identities', async ({ request }) => {
			posted = (await request.json()) as Record<string, unknown>
			const created = { id: identityID2, channel: 'phone', identifier: '+184467235', display_name: '' }
			identities.push(created)
			return HttpResponse.json(created, { status: 201 })
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByLabelText('Channel'))
	await userEvent.click(await screen.findByRole('option', { name: 'Phone' }))
	await userEvent.type(screen.getByLabelText('Value'), '+184 467 235')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByText('phone: +184467235')).toBeInTheDocument()
	expect(posted).toMatchObject({ channel: 'phone', identifier: '+184 467 235' })
})

test('names the owner when the identity belongs to someone else', async () => {
	server.use(
		http.post('/api/contacts/:id/identities', () =>
			HttpResponse.json(
				{
					error: 'contact: identity already exists',
					owner: contactRow(brunoID, 'Bruno'),
				},
				{ status: 409 },
			),
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
		http.post('/api/contacts/:id/identities', () =>
			HttpResponse.json({ error: 'contact: empty identifier' }, { status: 422 }),
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
		http.post('/api/contacts/:id/identities', () =>
			HttpResponse.json({ error: 'contact: identity already exists' }, { status: 409 }),
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

test('reports a generic conflict when the response body is unreadable', async () => {
	server.use(
		http.post(
			'/api/contacts/:id/identities',
			() => new HttpResponse('not json', { status: 409 }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'the identity already exists',
	)
})

test('reports a generic conflict when the body carries no message', async () => {
	server.use(
		http.post('/api/contacts/:id/identities', () =>
			HttpResponse.json({ oops: true }, { status: 409 }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'the identity already exists',
	)
})

test('reports a generic message when the identity add fails otherwise', async () => {
	server.use(
		http.post('/api/contacts/:id/identities', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The identity could not be added.',
	)
})

test('drops the session when the identity add is unauthorized', async () => {
	server.use(
		http.post('/api/contacts/:id/identities', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
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
		http.delete('/api/contacts/:id/identities/:identityId', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
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
	let deletedPath = ''
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: { contact: detailFor(String(variables.id), 'Ana García', identities) },
			}),
		),
		http.delete('/api/contacts/:id/identities/:identityId', ({ params, request }) => {
			deletedPath = new URL(request.url).pathname
			identities = identities.filter((identity) => identity.id !== params.identityId)
			return new HttpResponse(null, { status: 204 })
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByRole('button', { name: 'Remove maria@example.com' }))

	await waitFor(() =>
		expect(screen.queryByText('email: maria@example.com')).not.toBeInTheDocument(),
	)
	expect(deletedPath).toBe(`/api/contacts/${anaID}/identities/${identityID2}`)
	expect(screen.getByText('whatsapp: 184467235 (Ana G)')).toBeInTheDocument()
})

test('reports a failed removal', async () => {
	server.use(
		http.delete('/api/contacts/:id/identities/:identityId', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByRole('button', { name: 'Remove 184467235' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The identity could not be removed.',
	)
})

test('renames a contact', async () => {
	let currentName = 'Ana García'
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: { contact: detailFor(String(variables.id), currentName, []) },
			}),
		),
		http.patch('/api/contacts/:id', async ({ request, params }) => {
			const body = (await request.json()) as { name: string }
			currentName = body.name
			return HttpResponse.json(contactRow(String(params.id), currentName))
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
})

test('reports invalid contact details on rename', async () => {
	server.use(
		http.patch('/api/contacts/:id', () =>
			HttpResponse.json({ oops: true }, { status: 422 }),
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
		http.patch('/api/contacts/:id', () =>
			new HttpResponse('bad gateway', { status: 502 }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	const name = await screen.findByLabelText('Name')

	await userEvent.type(name, ' Ltd')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(
		await screen.findByText('The contact could not be renamed.'),
	).toBeInTheDocument()
})

test('surfaces the backend message for unreadable rename rejections', async () => {
	server.use(
		http.patch('/api/contacts/:id', () =>
			new HttpResponse('not json', { status: 422 }),
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
		http.patch('/api/contacts/:id', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
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



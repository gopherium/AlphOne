import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { renderAt } from './render'

const anaID = '0198c000-0000-7000-8000-000000000001'
const brunoID = '0198c000-0000-7000-8000-000000000002'
const carlaID = '0198c000-0000-7000-8000-000000000003'
const adaID = '0198c000-0000-7000-8000-000000000004'

function contactRow(id: string, name: string) {
	return { id, name, created_at: '2026-07-06T10:00:00Z' }
}

let listQueries: string[] = []

beforeEach(() => {
	listQueries = []
	server.use(
		http.get('/api/contacts', ({ request }) => {
			const url = new URL(request.url)
			const q = url.searchParams.get('q') ?? ''
			const cursor = url.searchParams.get('cursor') ?? ''
			listQueries.push(q)
			if (q === 'ada') {
				return HttpResponse.json({
					contacts: [contactRow(adaID, 'Ada Lovelace')],
					next_cursor: null,
				})
			}
			if (q !== '') {
				return HttpResponse.json({ contacts: [], next_cursor: null })
			}
			if (cursor === 'CUR1') {
				return HttpResponse.json({
					contacts: [contactRow(carlaID, 'Carla')],
					next_cursor: null,
				})
			}
			return HttpResponse.json({
				contacts: [contactRow(anaID, 'Ana García'), contactRow(brunoID, 'Bruno')],
				next_cursor: 'CUR1',
			})
		}),
		http.get('/api/contacts/:id', ({ params }) =>
			HttpResponse.json({
				...contactRow(String(params.id), 'Ana García'),
				identities: [
					{ channel: 'whatsapp', identifier: '184467235', display_name: 'Ana G' },
					{ channel: 'whatsapp', identifier: '184467236', display_name: '' },
				],
			}),
		),
	)
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
})

test('reports when contacts cannot be loaded', async () => {
	server.use(
		http.get('/api/contacts', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)

	renderAt('/contacts')

	expect(await screen.findByText(/could not be loaded/i)).toBeInTheDocument()
})

test('drops the session when the contacts request is unauthorized', async () => {
	server.use(
		http.get('/api/contacts', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
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
			HttpResponse.json(contactRow(newID, 'Nueva SL'), { status: 201 }),
		),
		http.get('/api/contacts/:id', () =>
			HttpResponse.json({ ...contactRow(newID, 'Nueva SL'), identities: [] }),
		),
	)
	renderAt('/contacts/new')
	const create = await screen.findByRole('button', { name: 'Create contact' })
	expect(create).toHaveAttribute('aria-disabled', 'true')

	await userEvent.type(screen.getByLabelText('Name'), 'Nueva SL')
	await userEvent.click(create)

	expect(
		await screen.findByRole('heading', { name: 'Nueva SL' }),
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

test('renames a contact', async () => {
	let currentName = 'Ana García'
	server.use(
		http.get('/api/contacts/:id', ({ params }) =>
			HttpResponse.json({
				...contactRow(String(params.id), currentName),
				identities: [],
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

	await userEvent.type(name, 'Ana García SL')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(
		await screen.findByRole('heading', { name: 'Ana García SL' }),
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

	await userEvent.type(name, ' SL')
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

	await userEvent.type(name, ' SL')
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

	await userEvent.type(name, ' SL')
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

	await userEvent.type(name, ' SL')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})

test('reports when the contact cannot be loaded', async () => {
	server.use(
		http.get('/api/contacts/:id', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)

	renderAt(`/contacts/${anaID}`)

	expect(await screen.findByText(/could not be loaded/i)).toBeInTheDocument()
})

test('drops the session when the contact detail is unauthorized', async () => {
	server.use(
		http.get('/api/contacts/:id', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
	)

	const client = renderAt(`/contacts/${anaID}`)

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})

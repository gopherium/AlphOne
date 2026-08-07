import { http, HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { renderAt } from './render'

const createdID = '0198c000-0000-7000-8000-000000000301'
const contactID = '0198c000-0000-7000-8000-000000000302'

// today is computed independently of the screen's own date helper.
const today = new Date().toLocaleDateString('en-CA')

let created: Record<string, unknown>[] = []
let searches: string[] = []

function searchPage(named: [string, string][]) {
	return {
		__typename: 'ContactConnection',
		edges: named.map(([id, name]) => ({
			__typename: 'ContactEdge',
			node: { __typename: 'Contact', id, name, createdAt: '2026-07-06T10:00:00Z' },
			cursor: id,
		})),
		pageInfo: { __typename: 'PageInfo', hasNextPage: false, endCursor: null },
	}
}

function taskBody(overrides: Record<string, unknown> = {}) {
	return {
		id: createdID,
		assignee_id: '0198c000-0000-7000-8000-0000000000aa',
		contact_id: null,
		title: 'Order more boxes',
		status: 'open',
		priority: 0,
		due_on: today,
		origin_source: null,
		origin_event_id: null,
		created_at: '2026-07-29T10:00:00Z',
		...overrides,
	}
}

beforeEach(() => {
	created = []
	searches = []
	server.use(
		http.post('/api/tasks', async ({ request }) => {
			const body = (await request.json()) as Record<string, unknown>
			created.push(body)
			return HttpResponse.json(taskBody(body), { status: 201 })
		}),
		graphql.query('TaskDetail', () =>
			HttpResponse.json({
				data: {
					task: {
						__typename: 'Task',
						id: createdID,
						title: String(created.at(-1)?.title ?? 'Order more boxes'),
						status: 'open',
						priority: 0,
						dueOn: today,
						contactId: null,
						contact: null,
					},
				},
			}),
		),
		graphql.query('Contacts', ({ variables }) => {
			const query = (variables.q as string | null) ?? ''
			searches.push(query)
			if (query === '') {
				return HttpResponse.json({ data: { contacts: searchPage([]) } })
			}
			return HttpResponse.json({
				data: { contacts: searchPage([[contactID, 'Maria Perez']]) },
			})
		}),
	)
})

test('offers the new task form from the day list', async () => {
	renderAt('/tasks')

	await userEvent.click(await screen.findByRole('link', { name: 'New task' }))

	expect(await screen.findByRole('heading', { name: 'New task' })).toBeInTheDocument()
})

test('starts the form on the day being viewed', async () => {
	renderAt('/tasks/new?date=2026-08-10')

	expect(await screen.findByLabelText('Due date')).toHaveValue('2026-08-10')
})

test('starts the form on today when no day is given', async () => {
	renderAt('/tasks/new')

	expect(await screen.findByLabelText('Due date')).toHaveValue(today)
})

test('creates a task and opens its detail', async () => {
	renderAt('/tasks/new')
	await userEvent.type(await screen.findByLabelText('Title'), 'Order more boxes')

	await userEvent.click(screen.getByRole('button', { name: 'Create task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toMatchObject({ title: 'Order more boxes', due_on: today, priority: 0 })
	expect(
		await screen.findByRole('heading', { name: 'Order more boxes' }),
	).toBeInTheDocument()
})

test('creates a task on a day chosen in the form', async () => {
	renderAt('/tasks/new')
	await userEvent.type(await screen.findByLabelText('Title'), 'Order more boxes')

	const due = screen.getByLabelText('Due date')
	await userEvent.clear(due)
	await userEvent.type(due, '2026-09-01')
	await userEvent.click(screen.getByRole('button', { name: 'Create task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toMatchObject({ due_on: '2026-09-01' })
})

test('creates a task with a raised priority', async () => {
	renderAt('/tasks/new')
	await userEvent.type(await screen.findByLabelText('Title'), 'Order more boxes')

	await userEvent.click(screen.getByLabelText('Priority'))
	await userEvent.click(await screen.findByRole('option', { name: 'High' }))
	await userEvent.click(screen.getByRole('button', { name: 'Create task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toMatchObject({ priority: 1 })
})

test('links the new task to a contact found by search', async () => {
	renderAt('/tasks/new')
	await userEvent.type(await screen.findByLabelText('Title'), 'Call her back')

	await userEvent.type(screen.getByLabelText('Link a contact'), 'maria')
	await userEvent.click(await screen.findByRole('button', { name: 'Maria Perez' }))
	await userEvent.click(screen.getByRole('button', { name: 'Create task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toMatchObject({ contact_id: contactID })
})

test('drops a linked contact before creating', async () => {
	renderAt('/tasks/new')
	await userEvent.type(await screen.findByLabelText('Title'), 'Call her back')
	await userEvent.type(screen.getByLabelText('Link a contact'), 'maria')
	await userEvent.click(await screen.findByRole('button', { name: 'Maria Perez' }))

	await userEvent.click(screen.getByRole('button', { name: 'Remove Maria Perez' }))
	await userEvent.click(screen.getByRole('button', { name: 'Create task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0].contact_id).toBeUndefined()
})

test('waits for the contact search to settle', async () => {
	renderAt('/tasks/new')

	await userEvent.type(await screen.findByLabelText('Link a contact'), 'maria')

	await screen.findByRole('button', { name: 'Maria Perez' })
	expect(searches).toContain('maria')
	expect(searches).not.toContain('m')
	expect(searches).not.toContain('ma')
})

test('says when a contact search finds nobody', async () => {
	server.use(
		graphql.query('Contacts', () =>
			HttpResponse.json({ data: { contacts: searchPage([]) } }),
		),
	)
	renderAt('/tasks/new')

	await userEvent.type(await screen.findByLabelText('Link a contact'), 'zz')

	expect(await screen.findByText('No contacts found.')).toBeInTheDocument()
})

test('refuses to create a task without a title', async () => {
	renderAt('/tasks/new')

	expect(await screen.findByRole('button', { name: 'Create task' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('reports when the task is rejected', async () => {
	server.use(
		http.post('/api/tasks', () =>
			HttpResponse.json({ error: 'task: empty title' }, { status: 422 }),
		),
	)
	renderAt('/tasks/new')

	await userEvent.type(await screen.findByLabelText('Title'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create task' }))

	expect(await screen.findByText('task: empty title')).toBeInTheDocument()
})

test('reports a generic message when creation fails otherwise', async () => {
	server.use(
		http.post('/api/tasks', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt('/tasks/new')

	await userEvent.type(await screen.findByLabelText('Title'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create task' }))

	expect(await screen.findByText('The task could not be created.')).toBeInTheDocument()
})

test('drops the session when creation is unauthorized', async () => {
	server.use(
		http.post('/api/tasks', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
	)
	const client = renderAt('/tasks/new')

	await userEvent.type(await screen.findByLabelText('Title'), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Create task' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('drops the session when the contact search is unauthorized', async () => {
	server.use(
		graphql.query('Contacts', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt('/tasks/new')

	await userEvent.type(await screen.findByLabelText('Link a contact'), 'maria')

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

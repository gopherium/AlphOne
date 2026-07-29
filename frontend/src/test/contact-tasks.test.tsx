import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { renderAt } from './render'

const contactID = '0198c000-0000-7000-8000-000000000401'
const callID = '0198c000-0000-7000-8000-000000000402'
const quoteID = '0198c000-0000-7000-8000-000000000403'

// These dates are computed independently of the screen's own date helper.
function localDate(offsetDays: number) {
	const at = new Date()
	at.setDate(at.getDate() + offsetDays)
	return at.toLocaleDateString('en-CA')
}

const today = localDate(0)
const tomorrow = localDate(1)

function taskRow(id: string, title: string, dueOn: string) {
	return {
		id,
		assignee_id: '0198c000-0000-7000-8000-0000000000aa',
		contact_id: contactID,
		title,
		status: 'open',
		priority: 0,
		due_on: dueOn,
		origin_source: null,
		origin_event_id: null,
		created_at: '2026-07-29T10:00:00Z',
	}
}

let contactFilters: string[] = []
let created: Record<string, unknown>[] = []
let patched: Record<string, unknown>[] = []
let tasks: ReturnType<typeof taskRow>[] = []

beforeEach(() => {
	contactFilters = []
	created = []
	patched = []
	tasks = [taskRow(callID, 'Call her back', today), taskRow(quoteID, 'Send the quote', tomorrow)]
	server.use(
		http.get('/api/contacts/:id', ({ params }) =>
			HttpResponse.json({
				id: String(params.id),
				name: 'Maria Perez',
				created_at: '2026-07-06T10:00:00Z',
				identities: [],
			}),
		),
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			const contact = url.searchParams.get('contact_id')
			if (contact === null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			contactFilters.push(contact)
			return HttpResponse.json({ tasks, next_cursor: null })
		}),
		http.post('/api/tasks', async ({ request }) => {
			const body = (await request.json()) as Record<string, unknown>
			created.push(body)
			const row = taskRow('0198c000-0000-7000-8000-000000000404', String(body.title), today)
			tasks = [...tasks, row]
			return HttpResponse.json(row, { status: 201 })
		}),
		http.patch('/api/tasks/:id', async ({ request, params }) => {
			const body = (await request.json()) as Record<string, unknown>
			patched.push({ id: String(params.id), ...body })
			tasks = tasks.filter((row) => row.id !== String(params.id))
			return HttpResponse.json({ ...taskRow(String(params.id), 'x', today), ...body })
		}),
	)
})

test('lists the open tasks of a contact with their due dates', async () => {
	renderAt(`/contacts/${contactID}`)

	const list = await screen.findByRole('list', { name: 'Contact tasks' })
	expect(within(list).getByText('Call her back')).toBeInTheDocument()
	expect(within(list).getByText(`Due ${tomorrow}`)).toBeInTheDocument()
	expect(contactFilters).toContain(contactID)
})

test('opens a task from the contact page', async () => {
	server.use(http.get('/api/tasks/:id', () => HttpResponse.json(taskRow(callID, 'Call her back', today))))
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.click(screen.getByRole('link', { name: 'Call her back' }))

	expect(await screen.findByRole('heading', { name: 'Call her back' })).toBeInTheDocument()
})

test('completes a task from the contact page', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('button', { name: 'Complete' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ id: callID, status: 'done' })
})

test('pushes a task to tomorrow from the contact page', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('button', { name: 'Push to tomorrow' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ id: callID, due_on: tomorrow })
})

test('adds a task for the contact due today', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'New task for this contact' }), 'Send the invoice')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toMatchObject({
		title: 'Send the invoice',
		contact_id: contactID,
		due_on: today,
	})
	expect(await screen.findByText('Send the invoice')).toBeInTheDocument()
})

test('says when a contact has nothing open', async () => {
	tasks = []

	renderAt(`/contacts/${contactID}`)

	expect(await screen.findByText('No open tasks.')).toBeInTheDocument()
})

test('loads more contact tasks through the cursor', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('contact_id') === null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			if (url.searchParams.get('cursor') === null) {
				return HttpResponse.json({
					tasks: [taskRow(callID, 'Call her back', today)],
					next_cursor: 'CUR1',
				})
			}
			return HttpResponse.json({
				tasks: [taskRow(quoteID, 'Send the quote', tomorrow)],
				next_cursor: null,
			})
		}),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByText('Call her back')

	await userEvent.click(screen.getByRole('button', { name: 'Load more tasks' }))

	expect(await screen.findByText('Send the quote')).toBeInTheDocument()
})

test('reports when the contact tasks cannot be loaded', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('contact_id') === null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			return HttpResponse.json({ error: 'internal error' }, { status: 500 })
		}),
	)

	renderAt(`/contacts/${contactID}`)

	expect(await screen.findByText('Tasks could not be loaded.')).toBeInTheDocument()
})

test('reports when a contact task cannot be added', async () => {
	server.use(
		http.post('/api/tasks', () =>
			HttpResponse.json({ error: 'task: empty title' }, { status: 422 }),
		),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'New task for this contact' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('task: empty title')).toBeInTheDocument()
})

test('disables the row control while its update is in flight', async () => {
	let release: (() => void) | undefined
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		http.patch('/api/tasks/:id', async ({ params }) => {
			await held
			return HttpResponse.json(taskRow(String(params.id), 'Call her back', today))
		}),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })
	const row = screen.getByRole('listitem', { name: 'Call her back' })

	await userEvent.click(within(row).getByRole('button', { name: 'Complete' }))

	await waitFor(() =>
		expect(within(row).getByRole('button', { name: 'Complete' })).toHaveAttribute(
			'aria-disabled',
			'true',
		),
	)
	release?.()
})

test('reports a generic message when adding fails otherwise', async () => {
	server.use(
		http.post('/api/tasks', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'New task for this contact' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('The task could not be added.')).toBeInTheDocument()
})

test('reports when a contact task cannot be updated', async () => {
	server.use(
		http.patch('/api/tasks/:id', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('button', { name: 'Push to tomorrow' }))

	expect(await screen.findByText('The task could not be updated.')).toBeInTheDocument()
})

test('drops the session when the contact tasks are unauthorized', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('contact_id') === null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			return HttpResponse.json({ error: 'no session' }, { status: 401 })
		}),
	)

	const client = renderAt(`/contacts/${contactID}`)

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

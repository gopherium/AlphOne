import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { renderAt } from './render'

const callID = '0198c000-0000-7000-8000-000000000101'
const quoteID = '0198c000-0000-7000-8000-000000000102'
const doneID = '0198c000-0000-7000-8000-000000000103'
const addedID = '0198c000-0000-7000-8000-000000000104'

// today is computed independently of the screen's own date helper.
const today = new Date().toLocaleDateString('en-CA')

function taskRow(id: string, title: string, status = 'open', priority = 0) {
	return {
		id,
		assignee_id: '0198c000-0000-7000-8000-0000000000aa',
		contact_id: null,
		title,
		status,
		priority,
		due_on: today,
		origin_source: null,
		origin_event_id: null,
		created_at: '2026-07-29T10:00:00Z',
	}
}

let listedDates: string[] = []
let tasks: ReturnType<typeof taskRow>[] = []

beforeEach(() => {
	listedDates = []
	tasks = [
		taskRow(callID, 'Call the supplier'),
		taskRow(quoteID, 'Send the quote'),
		taskRow(doneID, 'Book the courier', 'done'),
	]
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			listedDates.push(url.searchParams.get('date') ?? '')
			return HttpResponse.json({ tasks, next_cursor: null })
		}),
		http.patch('/api/tasks/:id', async ({ request, params }) => {
			const body = (await request.json()) as { status?: string }
			const stored = tasks.find((row) => row.id === String(params.id))
			if (stored === undefined) {
				return HttpResponse.json({ error: 'task: not found' }, { status: 404 })
			}
			const updated = { ...stored, status: body.status ?? stored.status }
			tasks = tasks.map((row) => (row.id === updated.id ? updated : row))
			return HttpResponse.json(updated)
		}),
		http.post('/api/tasks', async ({ request }) => {
			const body = (await request.json()) as { title: string }
			const created = taskRow(addedID, body.title)
			tasks = [...tasks, created]
			return HttpResponse.json(created, { status: 201 })
		}),
	)
})

test('serves the tasks screen at /tasks', async () => {
	renderAt('/tasks')

	expect(await screen.findByRole('heading', { name: 'Tasks' })).toBeInTheDocument()
	expect(await screen.findByText('Call the supplier')).toBeInTheDocument()
	expect(screen.getByText('Send the quote')).toBeInTheDocument()
})

test('asks the backend for today', async () => {
	renderAt('/tasks')

	await screen.findByText('Call the supplier')
	expect(listedDates).toContain(today)
})

test('navigates to the tasks screen from the main menu', async () => {
	renderAt('/')

	await userEvent.click(await screen.findByRole('link', { name: 'Tasks' }))

	expect(await screen.findByRole('heading', { name: 'Tasks' })).toBeInTheDocument()
})

test('keeps done tasks out of the open list until the group is opened', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	const open = screen.getByRole('list', { name: 'Open tasks' })

	expect(within(open).queryByText('Book the courier')).not.toBeInTheDocument()

	await userEvent.click(screen.getByRole('button', { name: 'Done (1)' }))

	expect(await screen.findByText('Book the courier')).toBeInTheDocument()
})

test('completes a task and moves it into the done group', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('button', { name: 'Complete' }))

	await waitFor(() =>
		expect(screen.getByRole('button', { name: 'Done (2)' })).toBeInTheDocument(),
	)
	const open = screen.getByRole('list', { name: 'Open tasks' })
	expect(within(open).queryByText('Call the supplier')).not.toBeInTheDocument()
})

test('reopens a done task', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	await userEvent.click(screen.getByRole('button', { name: 'Done (1)' }))

	const row = await screen.findByRole('listitem', { name: 'Book the courier' })
	await userEvent.click(within(row).getByRole('button', { name: 'Reopen' }))

	await waitFor(() => {
		const open = screen.getByRole('list', { name: 'Open tasks' })
		expect(within(open).getByText('Book the courier')).toBeInTheDocument()
	})
})

test('adds a task from the quick add field', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(
		screen.getByRole('textbox', { name: 'New task' }),
		'Order more boxes',
	)
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('Order more boxes')).toBeInTheDocument()
	expect(screen.getByRole('textbox', { name: 'New task' })).toHaveValue('')
})

test('does not add a task without a title', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	expect(screen.getByRole('button', { name: 'Add task' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('shows an empty state when the day has no tasks', async () => {
	server.use(
		http.get('/api/tasks', () => HttpResponse.json({ tasks: [], next_cursor: null })),
	)

	renderAt('/tasks')

	expect(await screen.findByText('Nothing due today.')).toBeInTheDocument()
})

test('loads more tasks through the cursor', async () => {
	let served = 0
	server.use(
		http.get('/api/tasks', () => {
			served += 1
			if (served === 1) {
				return HttpResponse.json({
					tasks: [taskRow(callID, 'Call the supplier')],
					next_cursor: 'CUR1',
				})
			}
			return HttpResponse.json({
				tasks: [taskRow(quoteID, 'Send the quote')],
				next_cursor: null,
			})
		}),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.click(screen.getByRole('button', { name: 'Load more' }))

	expect(await screen.findByText('Send the quote')).toBeInTheDocument()
})

test('reports when the tasks cannot be loaded', async () => {
	server.use(
		http.get('/api/tasks', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)

	renderAt('/tasks')

	expect(await screen.findByText(/could not be loaded/i)).toBeInTheDocument()
})

test('reports when a task cannot be added', async () => {
	server.use(
		http.post('/api/tasks', () =>
			HttpResponse.json({ error: 'task: empty title' }, { status: 422 }),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'New task' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('task: empty title')).toBeInTheDocument()
})

test('reports a generic message when adding fails otherwise', async () => {
	server.use(
		http.post('/api/tasks', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'New task' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('The task could not be added.')).toBeInTheDocument()
})

test('reports when a task cannot be updated', async () => {
	server.use(
		http.patch('/api/tasks/:id', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('button', { name: 'Complete' }))

	expect(await screen.findByText('The task could not be updated.')).toBeInTheDocument()
})

test('reports the backend message when an update is rejected', async () => {
	server.use(
		http.patch('/api/tasks/:id', () =>
			HttpResponse.json({ error: 'task: invalid status' }, { status: 422 }),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('button', { name: 'Complete' }))

	expect(await screen.findByText('The task could not be updated.')).toBeInTheDocument()
})

test('falls back to generic copy for an unreadable rejection', async () => {
	server.use(
		http.post('/api/tasks', () => new HttpResponse('not json', { status: 422 })),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'New task' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('invalid task details')).toBeInTheDocument()
})

test('falls back to generic copy for a rejection without a message', async () => {
	server.use(
		http.post('/api/tasks', () => HttpResponse.json({ oops: true }, { status: 422 })),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'New task' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('invalid task details')).toBeInTheDocument()
})

test('disables the row control while its update is in flight', async () => {
	let release: (() => void) | undefined
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		http.patch('/api/tasks/:id', async ({ params }) => {
			await held
			const stored = tasks.find((row) => row.id === String(params.id))
			return HttpResponse.json({ ...stored, status: 'done' })
		}),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	const row = screen.getByRole('listitem', { name: 'Call the supplier' })

	await userEvent.click(within(row).getByRole('button', { name: 'Complete' }))

	await waitFor(() =>
		expect(within(row).getByRole('button', { name: 'Complete' })).toHaveAttribute(
			'aria-disabled',
			'true',
		),
	)
	const other = screen.getByRole('listitem', { name: 'Send the quote' })
	expect(within(other).getByRole('button', { name: 'Complete' })).not.toHaveAttribute(
		'aria-disabled',
		'true',
	)
	release?.()
})

test('drops the session when the list is unauthorized', async () => {
	server.use(
		http.get('/api/tasks', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
	)

	const client = renderAt('/tasks')

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('drops the session when adding is unauthorized', async () => {
	server.use(
		http.post('/api/tasks', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
	)
	const client = renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'New task' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('drops the session when updating is unauthorized', async () => {
	server.use(
		http.patch('/api/tasks/:id', () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
	)
	const client = renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('button', { name: 'Complete' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

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
const oldID = '0198c000-0000-7000-8000-000000000105'

// These dates are computed independently of the screen's own date helpers.
function localDate(offsetDays: number) {
	const at = new Date()
	at.setDate(at.getDate() + offsetDays)
	return at.toLocaleDateString('en-CA')
}

function dueLabel(iso: string) {
	const [year, month, day] = iso.split('-').map(Number)
	return `Due ${new Date(year, month - 1, day).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}`
}

const today = localDate(0)
const tomorrow = localDate(1)
const yesterday = localDate(-1)

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
let overdueBefore: string[] = []
let patched: { id: string; body: Record<string, unknown> }[] = []
let tasks: ReturnType<typeof taskRow>[] = []
let overdue: ReturnType<typeof taskRow>[] = []

beforeEach(() => {
	listedDates = []
	overdueBefore = []
	patched = []
	overdue = []
	tasks = [
		taskRow(callID, 'Call the supplier'),
		taskRow(quoteID, 'Send the quote'),
		taskRow(doneID, 'Book the courier', 'done'),
	]
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			const dueBefore = url.searchParams.get('due_before')
			if (dueBefore !== null) {
				overdueBefore.push(dueBefore)
				return HttpResponse.json({ tasks: overdue, next_cursor: null })
			}
			listedDates.push(url.searchParams.get('date') ?? '')
			const status = url.searchParams.get('status') ?? 'open'
			const rows = status === 'all' ? tasks : tasks.filter((row) => row.status === status)
			return HttpResponse.json({ tasks: rows, next_cursor: null })
		}),
		http.patch('/api/tasks/:id', async ({ request, params }) => {
			const body = (await request.json()) as { status?: string; due_on?: string }
			patched.push({ id: String(params.id), body })
			const stored = [...tasks, ...overdue].find((row) => row.id === String(params.id))
			if (stored === undefined) {
				return HttpResponse.json({ error: 'task: not found' }, { status: 404 })
			}
			const updated = {
				...stored,
				status: body.status ?? stored.status,
				due_on: body.due_on ?? stored.due_on,
			}
			tasks = tasks.map((row) => (row.id === updated.id ? updated : row))
			overdue = overdue.filter((row) => row.id !== updated.id || body.due_on === undefined)
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

test('ghosts the rows while the day of tasks arrives', async () => {
	server.use(http.get('/api/tasks', () => new Promise(() => {})))
	renderAt('/tasks')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading tasks…')
	expect(status.closest('.godmin-loading-rows')).not.toBeNull()
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
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

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
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Reopen' }))

	await waitFor(() => {
		const open = screen.getByRole('list', { name: 'Open tasks' })
		expect(within(open).getByText('Book the courier')).toBeInTheDocument()
	})
})

test('marks a raised priority on the row', async () => {
	tasks = [taskRow(callID, 'Call the supplier', 'open', 1)]

	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })
	expect(within(row).getByText('High')).toBeInTheDocument()
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
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('due_before') !== null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			if (url.searchParams.get('cursor') === null) {
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

test('shows open work even when finished tasks fill the first page', async () => {
	const finished = [
		taskRow(doneID, 'Book the courier', 'done'),
		taskRow(oldID, 'File the customs form', 'done'),
	]
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('due_before') !== null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			switch (url.searchParams.get('status')) {
				case 'open':
					return HttpResponse.json({
						tasks: [taskRow(callID, 'Reply to the imported enquiry')],
						next_cursor: null,
					})
				case 'done':
					return HttpResponse.json({ tasks: finished, next_cursor: null })
				default:
					return HttpResponse.json({ tasks: finished, next_cursor: 'MORE' })
			}
		}),
	)

	renderAt('/tasks')

	expect(await screen.findByText('Reply to the imported enquiry')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Done (2)' })).toBeInTheDocument()
})

test('keeps the day usable when the done group cannot be loaded', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('due_before') !== null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			if (url.searchParams.get('status') === 'done') {
				return HttpResponse.json({ error: 'internal error' }, { status: 500 })
			}
			return HttpResponse.json({
				tasks: [taskRow(callID, 'Call the supplier')],
				next_cursor: null,
			})
		}),
	)

	renderAt('/tasks')

	expect(await screen.findByText('Call the supplier')).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: /^Done \(/ })).not.toBeInTheDocument()
})

test('loads more done tasks through their own cursor', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('due_before') !== null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			if (url.searchParams.get('status') !== 'done') {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			if (url.searchParams.get('cursor') === null) {
				return HttpResponse.json({
					tasks: [taskRow(doneID, 'Book the courier', 'done')],
					next_cursor: 'D1',
				})
			}
			return HttpResponse.json({
				tasks: [taskRow(oldID, 'File the customs form', 'done')],
				next_cursor: null,
			})
		}),
	)
	renderAt('/tasks')

	await userEvent.click(await screen.findByRole('button', { name: 'Done (1+)' }))
	await userEvent.click(screen.getByRole('button', { name: 'Load more done' }))

	expect(await screen.findByText('File the customs form')).toBeInTheDocument()
	expect(await screen.findByRole('button', { name: 'Done (2)' })).toBeInTheDocument()
})

test('shows a task created elsewhere when the live stream announces it', async () => {
	const { FakeEventSource } = await import('@alphone/frontend-sdk/testing')
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	tasks = [...tasks, taskRow(addedID, 'Created by an automation')]
	const source = FakeEventSource.instances.find((s) => s.url === '/api/events')
	if (!source) {
		throw new Error('no live stream subscription to /api/events')
	}
	source.emit()

	expect(await screen.findByText('Created by an automation')).toBeInTheDocument()
})

test('reports when the tasks cannot be loaded', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('due_before') !== null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			return HttpResponse.json({ error: 'internal error' }, { status: 500 })
		}),
	)

	renderAt('/tasks')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Tasks could not be loaded.',
	)
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
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

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
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

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

	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

	await waitFor(() =>
		expect(within(row).getByRole('checkbox', { name: 'Complete' })).toHaveAttribute(
			'aria-disabled',
			'true',
		),
	)
	const other = screen.getByRole('listitem', { name: 'Send the quote' })
	expect(within(other).getByRole('checkbox', { name: 'Complete' })).not.toHaveAttribute(
		'aria-disabled',
		'true',
	)
	release?.()
})

test('lists the day named by the date search param', async () => {
	renderAt(`/tasks?date=${tomorrow}`)

	await screen.findByText('Call the supplier')
	expect(listedDates).toContain(tomorrow)
	expect(listedDates).not.toContain(today)
})

test('falls back to today when the date search param is unusable', async () => {
	renderAt('/tasks?date=not-a-date')

	await screen.findByText('Call the supplier')
	expect(listedDates).toContain(today)
})

test('walks to the next and previous day', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			const date = url.searchParams.get('date')
			if (date === null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			return HttpResponse.json({
				tasks: [taskRow(callID, `Work due ${date}`)],
				next_cursor: null,
			})
		}),
	)
	renderAt(`/tasks?date=${tomorrow}`)
	await screen.findByText(`Work due ${tomorrow}`)

	await userEvent.click(screen.getByRole('button', { name: 'Next day' }))

	expect(await screen.findByText(`Work due ${localDate(2)}`)).toBeInTheDocument()

	await userEvent.click(screen.getByRole('button', { name: 'Previous day' }))

	expect(await screen.findByText(`Work due ${tomorrow}`)).toBeInTheDocument()
})

test('offers a way back to today only when looking at another day', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	expect(screen.queryByRole('button', { name: 'Today' })).not.toBeInTheDocument()

	await userEvent.click(screen.getByRole('button', { name: 'Next day' }))
	await waitFor(() => expect(listedDates).toContain(tomorrow))
	await userEvent.click(await screen.findByRole('button', { name: 'Today' }))

	await waitFor(() =>
		expect(screen.queryByRole('button', { name: 'Today' })).not.toBeInTheDocument(),
	)
})

test('shows overdue work above the day, with its original due date', async () => {
	overdue = [{ ...taskRow(oldID, 'Chase the invoice'), due_on: yesterday }]

	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Chase the invoice' })
	expect(within(row).getByText(dueLabel(yesterday))).toBeInTheDocument()
	expect(overdueBefore).toContain(today)
})

test('keeps overdue work out of other days', async () => {
	overdue = [{ ...taskRow(oldID, 'Chase the invoice'), due_on: yesterday }]

	renderAt(`/tasks?date=${tomorrow}`)

	await screen.findByText('Call the supplier')
	expect(screen.queryByText('Chase the invoice')).not.toBeInTheDocument()
	expect(overdueBefore).toHaveLength(0)
})

test('says nothing about overdue work when there is none', async () => {
	renderAt('/tasks')

	await screen.findByText('Call the supplier')
	expect(screen.queryByRole('list', { name: 'Overdue tasks' })).not.toBeInTheDocument()
})

test('pushes a task to tomorrow', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('button', { name: 'Push to tomorrow' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toEqual({ id: callID, body: { due_on: tomorrow } })
})

test('pushes an overdue task to tomorrow rather than to the day after it was due', async () => {
	overdue = [{ ...taskRow(oldID, 'Chase the invoice'), due_on: localDate(-5) }]
	renderAt('/tasks')
	const row = await screen.findByRole('listitem', { name: 'Chase the invoice' })

	await userEvent.click(within(row).getByRole('button', { name: 'Push to tomorrow' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toEqual({ id: oldID, body: { due_on: tomorrow } })
})

test('pushes a task on a future day to the day after it', async () => {
	tasks = [{ ...taskRow(callID, 'Call the supplier'), due_on: tomorrow }]
	renderAt(`/tasks?date=${tomorrow}`)
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('button', { name: 'Push to tomorrow' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toEqual({ id: callID, body: { due_on: localDate(2) } })
})

test('loads more overdue work through the cursor', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('due_before') === null) {
				return HttpResponse.json({ tasks: [], next_cursor: null })
			}
			if (url.searchParams.get('cursor') === null) {
				return HttpResponse.json({
					tasks: [{ ...taskRow(oldID, 'Chase the invoice'), due_on: yesterday }],
					next_cursor: 'CUR1',
				})
			}
			return HttpResponse.json({
				tasks: [{ ...taskRow(callID, 'Refile the paperwork'), due_on: yesterday }],
				next_cursor: null,
			})
		}),
	)
	renderAt('/tasks')
	await screen.findByText('Chase the invoice')

	await userEvent.click(screen.getByRole('button', { name: 'Load more overdue' }))

	expect(await screen.findByText('Refile the paperwork')).toBeInTheDocument()
})

test('leaves done tasks without a push control', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	await userEvent.click(screen.getByRole('button', { name: 'Done (1)' }))

	const row = await screen.findByRole('listitem', { name: 'Book the courier' })
	expect(within(row).queryByRole('button', { name: 'Push to tomorrow' })).not.toBeInTheDocument()
})

test('reports when overdue work cannot be loaded', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('due_before') !== null) {
				return HttpResponse.json({ error: 'internal error' }, { status: 500 })
			}
			return HttpResponse.json({ tasks, next_cursor: null })
		}),
	)

	renderAt('/tasks')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Overdue tasks could not be loaded.',
	)
})

test('drops the session when the overdue list is unauthorized', async () => {
	server.use(
		http.get('/api/tasks', ({ request }) => {
			const url = new URL(request.url)
			if (url.searchParams.get('due_before') !== null) {
				return HttpResponse.json({ error: 'no session' }, { status: 401 })
			}
			return HttpResponse.json({ tasks, next_cursor: null })
		}),
	)

	const client = renderAt('/tasks')

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
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
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

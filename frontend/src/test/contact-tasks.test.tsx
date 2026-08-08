import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
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

function dueLabel(iso: string) {
	const [year, month, day] = iso.split('-').map(Number)
	return `Due ${new Date(year, month - 1, day).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}`
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

function taskNode(row: ReturnType<typeof taskRow>) {
	return {
		__typename: 'Task',
		id: row.id,
		title: row.title,
		status: row.status,
		priority: row.priority,
		dueOn: row.due_on,
	}
}

function contactDetail(id: string, rows: ReturnType<typeof taskRow>[], endCursor: string | null = null) {
	return {
		__typename: 'Contact',
		id,
		name: 'Maria Perez',
		createdAt: '2026-07-06T10:00:00Z',
		identities: [],
		tasks: {
			__typename: 'TaskConnection',
			edges: rows.map((row) => ({
				__typename: 'TaskEdge',
				node: {
					__typename: 'Task',
					id: row.id,
					title: row.title,
					status: row.status,
					priority: row.priority,
					dueOn: row.due_on,
				},
				cursor: row.id,
			})),
			pageInfo: { __typename: 'PageInfo', hasNextPage: endCursor !== null, endCursor },
		},
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
		graphql.query('ContactDetail', ({ variables }) => {
			contactFilters.push(String(variables.id))
			return HttpResponse.json({ data: { contact: contactDetail(String(variables.id), tasks) } })
		}),
		graphql.mutation('CreateTask', ({ variables }) => {
			const input = variables.input as Record<string, unknown>
			created.push({ title: input.title, due_on: input.dueOn, contact_id: input.contactId })
			const row = taskRow('0198c000-0000-7000-8000-000000000404', String(input.title), today)
			tasks = [...tasks, row]
			return HttpResponse.json({
				data: { createTask: { __typename: 'CreateTaskPayload', task: taskNode(row), replay: false } },
			})
		}),
		graphql.mutation('UpdateTask', ({ variables }) => {
			const input = variables.input as { status?: string; dueOn?: string }
			const id = String(variables.id)
			patched.push({ id, status: input.status, due_on: input.dueOn })
			tasks = tasks.filter((row) => row.id !== id)
			return HttpResponse.json({
				data: {
					updateTask: taskNode({
						...taskRow(id, 'x', input.dueOn ?? today),
						status: input.status ?? 'open',
					}),
				},
			})
		}),
	)
})

test('keeps the page chrome while the contact is on its way', async () => {
	server.use(graphql.query('ContactDetail', () => new Promise(() => {})))
	renderAt(`/contacts/${contactID}`)

	expect(await screen.findByRole('heading', { level: 1, name: 'Contact' })).toBeInTheDocument()
	const status = screen.getByRole('status')
	expect(status).toHaveTextContent('Loading contact…')
	expect(status.closest('.godmin-loading-screen')).not.toBeNull()
})

test('lists the open tasks of a contact with their due dates', async () => {
	renderAt(`/contacts/${contactID}`)

	const list = await screen.findByRole('list', { name: 'Contact tasks' })
	expect(within(list).getByText('Call her back')).toBeInTheDocument()
	expect(within(list).getByText(dueLabel(tomorrow))).toBeInTheDocument()
	expect(contactFilters).toContain(contactID)
})

test('opens a task from the contact page', async () => {
	server.use(
		graphql.query('TaskDetail', () =>
			HttpResponse.json({
				data: {
					task: {
						__typename: 'Task',
						id: callID,
						title: 'Call her back',
						status: 'open',
						priority: 0,
						dueOn: today,
						contactId: null,
						contact: null,
					},
				},
			}),
		),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.click(screen.getByRole('link', { name: 'Call her back' }))

	expect(await screen.findByRole('heading', { name: 'Call her back' })).toBeInTheDocument()
})

test('completes a task from the contact page', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

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
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: {
					contact: variables.after
						? contactDetail(String(variables.id), [taskRow(quoteID, 'Send the quote', tomorrow)])
						: contactDetail(String(variables.id), [taskRow(callID, 'Call her back', today)], 'CUR1'),
				},
			}),
		),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByText('Call her back')

	await userEvent.click(screen.getByRole('button', { name: 'Load more tasks' }))

	expect(await screen.findByText('Send the quote')).toBeInTheDocument()
	expect(screen.getByText('Call her back')).toBeInTheDocument()
})

test('reports when the contact detail cannot be loaded', async () => {
	server.use(
		graphql.query('ContactDetail', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	renderAt(`/contacts/${contactID}`)

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The contact could not be loaded.',
	)
})

test('reports when a contact task cannot be added', async () => {
	server.use(
		graphql.mutation('CreateTask', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'task: empty title', extensions: { code: 'VALIDATION' } }],
			}),
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
		graphql.mutation('UpdateTask', async ({ variables }) => {
			await held
			return HttpResponse.json({
				data: {
					updateTask: taskNode(taskRow(String(variables.id), 'Call her back', today)),
				},
			})
		}),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })
	const row = screen.getByRole('listitem', { name: 'Call her back' })

	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

	await waitFor(() =>
		expect(within(row).getByRole('checkbox', { name: 'Complete' })).toHaveAttribute(
			'aria-disabled',
			'true',
		),
	)
	release?.()
})

test('reports a generic message when adding fails otherwise', async () => {
	server.use(
		graphql.mutation('CreateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
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
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('button', { name: 'Push to tomorrow' }))

	expect(await screen.findByText('The task could not be updated.')).toBeInTheDocument()
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

	const client = renderAt(`/contacts/${contactID}`)

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('leaves the list alone when completing a contact task fails', async () => {
	server.use(
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

	expect(await screen.findByText('The task could not be updated.')).toBeInTheDocument()
	expect(screen.getByText('Call her back')).toBeInTheDocument()
})

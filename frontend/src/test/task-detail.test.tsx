import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { renderAt } from './render'

const taskID = '0198c000-0000-7000-8000-000000000201'
const contactID = '0198c000-0000-7000-8000-000000000202'

function taskDetail() {
	return {
		__typename: 'Task',
		id: String(stored.id),
		title: String(stored.title),
		status: String(stored.status),
		priority: Number(stored.priority),
		dueOn: String(stored.due_on),
		contactId: stored.contact_id === null ? null : String(stored.contact_id),
		contact:
			stored.contact_id === null
				? null
				: { __typename: 'Contact', id: String(stored.contact_id), name: 'Maria Perez' },
	}
}

let stored: Record<string, unknown>
let patched: Record<string, unknown>[] = []

beforeEach(() => {
	patched = []
	stored = {
		id: taskID,
		assignee_id: '0198c000-0000-7000-8000-0000000000aa',
		contact_id: null,
		title: 'Call the supplier',
		status: 'open',
		priority: 0,
		due_on: '2026-08-10',
		origin_source: null,
		origin_event_id: null,
		created_at: '2026-07-29T10:00:00Z',
	}
	server.use(
		graphql.query('TaskDetail', () => HttpResponse.json({ data: { task: taskDetail() } })),
		graphql.mutation('UpdateTask', ({ variables }) => {
			const input = variables.input as Record<string, unknown>
			patched.push({
				...(input.title === undefined ? {} : { title: input.title }),
				...(input.dueOn === undefined ? {} : { due_on: input.dueOn }),
				...(input.status === undefined ? {} : { status: input.status }),
				...(input.priority === undefined ? {} : { priority: input.priority }),
			})
			stored = {
				...stored,
				...(input.title === undefined ? {} : { title: input.title }),
				...(input.dueOn === undefined ? {} : { due_on: input.dueOn }),
				...(input.status === undefined ? {} : { status: input.status }),
				...(input.priority === undefined ? {} : { priority: input.priority }),
			}
			return HttpResponse.json({ data: { updateTask: taskDetail() } })
		}),

	)
})

test('keeps the page chrome while the task is on its way', async () => {
	server.use(graphql.query('TaskDetail', () => new Promise(() => {})))
	renderAt(`/tasks/${taskID}`)

	expect(await screen.findByRole('heading', { level: 1, name: 'Task' })).toBeInTheDocument()
	const status = screen.getByRole('status')
	expect(status).toHaveTextContent('Loading task…')
	expect(status.closest('.godmin-loading-screen')).not.toBeNull()
})

test('shows the task with its due date and priority', async () => {
	renderAt(`/tasks/${taskID}`)

	expect(await screen.findByRole('heading', { name: 'Call the supplier' })).toBeInTheDocument()
	expect(screen.getByLabelText('Title')).toHaveValue('Call the supplier')
	expect(screen.getByLabelText('Due date')).toHaveValue('2026-08-10')
	expect(screen.getByLabelText('Priority')).toHaveTextContent('Normal')
})

test('shows an unmapped priority as normal', async () => {
	stored = { ...stored, priority: 5 }

	renderAt(`/tasks/${taskID}`)

	expect(await screen.findByLabelText('Priority')).toHaveTextContent('Normal')
})

test('renames a task', async () => {
	renderAt(`/tasks/${taskID}`)
	const title = await screen.findByLabelText('Title')

	await userEvent.clear(title)
	await userEvent.type(title, 'Call the supplier back')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ title: 'Call the supplier back' })
	expect(
		await screen.findByRole('heading', { name: 'Call the supplier back' }),
	).toBeInTheDocument()
})

test('reschedules a task from the date field', async () => {
	renderAt(`/tasks/${taskID}`)
	const due = await screen.findByLabelText('Due date')

	await userEvent.clear(due)
	await userEvent.type(due, '2026-08-20')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ due_on: '2026-08-20' })
})

test('raises the priority of a task', async () => {
	renderAt(`/tasks/${taskID}`)

	await userEvent.click(await screen.findByLabelText('Priority'))
	await userEvent.click(await screen.findByRole('option', { name: 'High' }))
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ priority: 1 })
})

test('completes and reopens a task from its detail', async () => {
	renderAt(`/tasks/${taskID}`)

	await userEvent.click(await screen.findByRole('button', { name: 'Complete' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ status: 'done' })

	await userEvent.click(await screen.findByRole('button', { name: 'Reopen' }))

	await waitFor(() => expect(patched).toHaveLength(2))
	expect(patched[1]).toMatchObject({ status: 'open' })
})

test('links a task to the contact it belongs to', async () => {
	stored = { ...stored, contact_id: contactID }

	renderAt(`/tasks/${taskID}`)

	const link = await screen.findByRole('link', { name: 'Maria Perez' })
	expect(link).toHaveAttribute('href', `/contacts/${contactID}`)
})

test('shows no contact link for an unlinked task', async () => {
	renderAt(`/tasks/${taskID}`)

	await screen.findByRole('heading', { name: 'Call the supplier' })
	expect(screen.queryByRole('link', { name: 'Maria Perez' })).not.toBeInTheDocument()
})

test('reaches the detail from the day list', async () => {
	server.use(
		graphql.query('DayTasks', ({ variables }) =>
			HttpResponse.json({
				data: {
					tasks: {
						__typename: 'TaskConnection',
						edges: variables.status === 'open'
							? [{
								__typename: 'TaskEdge',
								node: {
									__typename: 'Task',
									id: taskID,
									title: 'Call the supplier',
									status: 'open',
									priority: 0,
									dueOn: '2026-08-10',
								},
								cursor: taskID,
							}]
							: [],
						pageInfo: { __typename: 'PageInfo', hasNextPage: false, endCursor: null },
					},
				},
			}),
		),
	)
	renderAt('/tasks')

	await userEvent.click(await screen.findByRole('link', { name: 'Call the supplier' }))

	expect(
		await screen.findByRole('heading', { name: 'Call the supplier' }),
	).toBeInTheDocument()
})

test('reports when the task cannot be loaded', async () => {
	server.use(
		graphql.query('TaskDetail', () =>
			HttpResponse.json({
				data: { task: null },
				errors: [{ message: 'task: not found', extensions: { code: 'NOT_FOUND' } }],
			}),
		),
	)

	renderAt(`/tasks/${taskID}`)

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The task could not be loaded.',
	)
})

test('reports when a save is rejected', async () => {
	server.use(
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'task: empty title', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt(`/tasks/${taskID}`)
	const title = await screen.findByLabelText('Title')

	await userEvent.type(title, ' back')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByText('task: empty title')).toBeInTheDocument()
})

test('reports a generic message when a save fails otherwise', async () => {
	server.use(
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt(`/tasks/${taskID}`)
	const title = await screen.findByLabelText('Title')

	await userEvent.type(title, ' back')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByText('The task could not be saved.')).toBeInTheDocument()
})

test('reports when completing from the detail fails', async () => {
	server.use(
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt(`/tasks/${taskID}`)

	await userEvent.click(await screen.findByRole('button', { name: 'Complete' }))

	expect(await screen.findByText('The task could not be saved.')).toBeInTheDocument()
})

test('drops the session when the task detail is unauthorized', async () => {
	server.use(
		graphql.query('TaskDetail', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)

	const client = renderAt(`/tasks/${taskID}`)

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

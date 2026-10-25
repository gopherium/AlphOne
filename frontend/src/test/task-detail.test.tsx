// SPDX-License-Identifier: AGPL-3.0-or-later

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
		dueTime: stored.due_time === null ? null : String(stored.due_time),
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
		contact_id: null,
		title: 'Call the supplier',
		status: 'open',
		priority: 0,
		due_on: '2026-08-10',
		due_time: null,
	}
	server.use(
		graphql.query('TaskDetail', () => HttpResponse.json({ data: { task: taskDetail() } })),
		graphql.mutation('UpdateTask', ({ variables }) => {
			const input = variables.input as Record<string, unknown>
			const changes = {
				...(input.title === undefined ? {} : { title: input.title }),
				...(input.dueOn === undefined ? {} : { due_on: input.dueOn }),
				...('dueTime' in input ? { due_time: input.dueTime } : {}),
				...(input.status === undefined ? {} : { status: input.status }),
				...(input.priority === undefined ? {} : { priority: input.priority }),
			}
			patched.push(changes)
			stored = { ...stored, ...changes }
			return HttpResponse.json({ data: { updateTask: taskDetail() } })
		}),

	)
})

test('keeps the page chrome while the task is on its way', async () => {
	server.use(graphql.query('TaskDetail', () => new Promise(() => {})))
	renderAt(`/tasks/${taskID}`)

	expect(await screen.findByRole('heading', { level: 2, name: 'Task' })).toBeInTheDocument()
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
	expect(screen.getByText('Task saved.')).toBeInTheDocument()
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

test('keeps Save off while the due date is empty', async () => {
	renderAt(`/tasks/${taskID}`)

	await userEvent.clear(await screen.findByLabelText('Due date'))

	expect(screen.getByRole('button', { name: 'Save' })).toHaveAttribute('aria-disabled', 'true')
})

test('shows the time of a task in the Time field next to the due date', async () => {
	stored = { ...stored, due_time: '09:30' }

	renderAt(`/tasks/${taskID}`)

	const time = await screen.findByLabelText('Time')
	expect(time).toHaveValue('09:30')
	expect(time).toHaveAttribute('type', 'time')
	const due = screen.getByLabelText('Due date')
	expect(due.compareDocumentPosition(time) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
	expect(time.compareDocumentPosition(screen.getByLabelText('Priority')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('leaves the Time field empty on a task with no time', async () => {
	renderAt(`/tasks/${taskID}`)

	expect(await screen.findByLabelText('Time')).toHaveValue('')
})

test('sets a time on a task that had none', async () => {
	renderAt(`/tasks/${taskID}`)

	await userEvent.type(await screen.findByLabelText('Time'), '14:15')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ due_time: '14:15' })
	expect(await screen.findByText('Task saved.')).toBeInTheDocument()
})

test('changes the time of a task', async () => {
	stored = { ...stored, due_time: '09:30' }
	renderAt(`/tasks/${taskID}`)
	const time = await screen.findByLabelText('Time')

	await userEvent.clear(time)
	await userEvent.type(time, '11:00')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ due_time: '11:00' })
})

test('clears the time of a task the reader empties, sending a null time', async () => {
	stored = { ...stored, due_time: '09:30' }
	renderAt(`/tasks/${taskID}`)

	await userEvent.clear(await screen.findByLabelText('Time'))
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toHaveProperty('due_time', null)
	await waitFor(() => expect(screen.getByLabelText('Time')).toHaveValue(''))
})

test('keeps the time of a task a save leaves alone', async () => {
	stored = { ...stored, due_time: '09:30' }
	renderAt(`/tasks/${taskID}`)
	const title = await screen.findByLabelText('Title')

	await userEvent.type(title, ' back')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ title: 'Call the supplier back', due_time: '09:30' })
})

test('sends no time for a task that had none when the field stays empty', async () => {
	renderAt(`/tasks/${taskID}`)
	const title = await screen.findByLabelText('Title')

	await userEvent.type(title, ' back')
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).not.toHaveProperty('due_time')
})

test('shows a time changed elsewhere once the task is read again', async () => {
	stored = { ...stored, due_time: '09:30' }
	renderAt(`/tasks/${taskID}`)
	expect(await screen.findByLabelText('Time')).toHaveValue('09:30')
	stored = { ...stored, due_time: '11:00' }

	await userEvent.click(screen.getByRole('button', { name: 'Complete' }))

	await waitFor(() => expect(screen.getByLabelText('Time')).toHaveValue('11:00'))
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
	expect(await screen.findByText('Task completed.')).toBeInTheDocument()

	await userEvent.click(await screen.findByRole('button', { name: 'Reopen' }))

	await waitFor(() => expect(patched).toHaveLength(2))
	expect(patched[1]).toMatchObject({ status: 'open' })
	expect(await screen.findByText('Task reopened.')).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Undo' })).not.toBeInTheDocument()
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
									dueTime: null,
									contact: null,
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
	expect(screen.queryByText('Task saved.')).not.toBeInTheDocument()
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
	expect(screen.queryByText('Task completed.')).not.toBeInTheDocument()
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

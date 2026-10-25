// SPDX-License-Identifier: AGPL-3.0-or-later

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
	return `Due ${iso.split('-').reverse().join('/')}`
}

/**
 * Returns the toast a postpone to the given day raises.
 * @param iso - The day the task moved to as YYYY-MM-DD.
 * @returns The toast text.
 */
function movedToast(iso: string) {
	return `Task moved to ${iso.split('-').reverse().join('/')}.`
}

const today = localDate(0)
const tomorrow = localDate(1)

function taskRow(id: string, title: string, dueOn: string) {
	return {
		id,
		contact_id: contactID,
		title,
		status: 'open',
		priority: 0,
		due_on: dueOn,
		due_time: null as string | null,
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
		dueTime: row.due_time,
	}
}

/**
 * Orders two open tasks as the server lists them, by priority, day, then time with an untimed one after, then creation.
 * @param first - One task.
 * @param second - The other task.
 * @returns A negative number when the first task lists first.
 */
function serverOrder(first: ReturnType<typeof taskRow>, second: ReturnType<typeof taskRow>) {
	const clock = (row: ReturnType<typeof taskRow>) => row.due_time ?? '24:00'
	return (
		second.priority - first.priority ||
		first.due_on.localeCompare(second.due_on) ||
		clock(first).localeCompare(clock(second)) ||
		first.id.localeCompare(second.id)
	)
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
					dueTime: row.due_time,
					contact: { __typename: 'Contact', id, name: 'Maria Perez' },
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
			created.push({
				title: input.title,
				due_on: input.dueOn,
				contact_id: input.contactId,
				priority: input.priority,
				...('dueTime' in input ? { due_time: input.dueTime } : {}),
			})
			const row = {
				...taskRow('0198c000-0000-7000-8000-000000000404', String(input.title), String(input.dueOn)),
				priority: Number(input.priority ?? 0),
				due_time: typeof input.dueTime === 'string' ? input.dueTime : null,
			}
			tasks = [...tasks, row]
			return HttpResponse.json({
				data: { createTask: { __typename: 'CreateTaskPayload', task: taskNode(row), replay: false } },
			})
		}),
		graphql.mutation('UpdateTask', ({ variables }) => {
			const input = variables.input as { status?: string; dueOn?: string }
			const id = String(variables.id)
			const timed = 'dueTime' in input ? { due_time: input.dueTime } : {}
			patched.push({ id, status: input.status, due_on: input.dueOn, ...timed })
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

	expect(await screen.findByRole('heading', { level: 2, name: 'Contact' })).toBeInTheDocument()
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

test('shows the time of a contact task before its title and keeps its due date after it', async () => {
	tasks = [
		{ ...taskRow(callID, 'Call her back', today), due_time: '09:30' },
		taskRow(quoteID, 'Send the quote', tomorrow),
	]

	renderAt(`/contacts/${contactID}`)

	const row = await screen.findByRole('listitem', { name: 'Call her back' })
	const time = within(row).getByText('09:30')
	const title = row.querySelector('.alphone-tasks__title') as Element
	const due = within(row).getByText(dueLabel(today))
	expect(time.tagName).toBe('TIME')
	expect(time).toHaveClass('alphone-tasks__time')
	expect(row.querySelector('.alphone-tasks__check')?.compareDocumentPosition(time)).toBe(
		Node.DOCUMENT_POSITION_FOLLOWING,
	)
	expect(time.nextElementSibling).toBe(title)
	expect(time.parentElement).toHaveClass('alphone-tasks__lead')
	expect(time.parentElement?.parentElement).toBe(row)
	expect(title.compareDocumentPosition(due) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('leaves the time column blank on a contact task with no time, as wide as a timed one', async () => {
	tasks = [
		{ ...taskRow(callID, 'Call her back', today), due_time: '09:30' },
		taskRow(quoteID, 'Send the quote', tomorrow),
	]

	renderAt(`/contacts/${contactID}`)

	const timed = (await screen.findByRole('listitem', { name: 'Call her back' })).querySelector('.alphone-tasks__time')
	const untimed = screen.getByRole('listitem', { name: 'Send the quote' }).querySelector('.alphone-tasks__time')
	expect(untimed).toBeEmptyDOMElement()
	expect(untimed?.nextElementSibling).toHaveClass('alphone-tasks__title')
	expect(untimed?.className).toBe(timed?.className)
	expect(untimed?.getAttribute('data-morning')).toBe(timed?.getAttribute('data-morning'))
	expect(untimed?.getAttribute('data-evening')).toBe(timed?.getAttribute('data-evening'))
})

test('keeps the contact tasks in the order the server lists them, each row with its time', async () => {
	tasks = [
		taskRow(callID, 'Call her back', today),
		{ ...taskRow(quoteID, 'Send the quote', today), due_time: '11:00' },
		{ ...taskRow('0198c000-0000-7000-8000-000000000405', 'Book the visit', today), due_time: '09:00' },
	]
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({ data: { contact: contactDetail(String(variables.id), tasks.toSorted(serverOrder)) } }),
		),
	)

	renderAt(`/contacts/${contactID}`)

	const list = await screen.findByRole('list', { name: 'Contact tasks' })
	const rows = within(list).getAllByRole('listitem')
	const titles = rows.map((row) => row.getAttribute('aria-label'))
	expect(titles).toEqual(['Book the visit', 'Send the quote', 'Call her back'])
	expect(rows.map((row) => row.querySelector('.alphone-tasks__time')?.textContent)).toEqual(['09:00', '11:00', ''])
})

test('leaves the contact name off the tasks on their own contact page', async () => {
	renderAt(`/contacts/${contactID}`)

	const list = await screen.findByRole('list', { name: 'Contact tasks' })
	expect(within(list).getByRole('link', { name: 'Call her back' })).toBeInTheDocument()
	expect(within(list).queryByRole('link', { name: 'Maria Perez' })).not.toBeInTheDocument()
	expect(within(list).getAllByRole('link')).toHaveLength(2)
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
						dueTime: null,
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
	expect(await screen.findByText('Task completed.')).toBeInTheDocument()
})

test('undoes a completion on the contact page from its toast', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })
	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))
	await screen.findByText('Task completed.')

	await userEvent.click(screen.getByRole('button', { name: 'Undo' }))

	await waitFor(() => expect(patched).toHaveLength(2))
	expect(patched[1]).toMatchObject({ id: callID, status: 'open' })
})

test('rereads the task list that is open when an undo lands from another screen', async () => {
	let dayReads = 0
	server.use(
		graphql.query('DayTasks', () => {
			dayReads++
			return HttpResponse.json({
				data: {
					tasks: {
						__typename: 'TaskConnection',
						edges: [],
						pageInfo: { __typename: 'PageInfo', hasNextPage: false, endCursor: null },
					},
				},
			})
		}),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })
	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))
	await screen.findByText('Task completed.')
	await userEvent.click(screen.getByRole('link', { name: 'Tasks' }))
	await waitFor(() => expect(dayReads).toBeGreaterThan(0))
	const before = dayReads

	await userEvent.click(screen.getByRole('button', { name: 'Undo' }))

	await waitFor(() => expect(dayReads).toBeGreaterThan(before))
})

test('pushes a task to tomorrow from the contact page', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('button', { name: 'Postpone' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toMatchObject({ id: callID, due_on: tomorrow })
	expect(await screen.findByText(movedToast(tomorrow))).toBeInTheDocument()
})

test('undoes a postpone on the contact page from its toast', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })
	const row = screen.getByRole('listitem', { name: 'Call her back' })
	await userEvent.click(within(row).getByRole('button', { name: 'Postpone' }))
	await screen.findByText(movedToast(tomorrow))

	await userEvent.click(screen.getByRole('button', { name: 'Undo' }))

	await waitFor(() => expect(patched).toHaveLength(2))
	expect(patched[1]).toMatchObject({ id: callID, due_on: today })
})

test('adds a task for the contact due today', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Send the invoice')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toMatchObject({
		title: 'Send the invoice',
		contact_id: contactID,
		due_on: today,
		priority: 0,
	})
	expect(await screen.findByText('Send the invoice')).toBeInTheDocument()
	expect(screen.getByText('Task added.')).toBeInTheDocument()
	await waitFor(() => expect(screen.getByRole('textbox', { name: 'Task title' })).toHaveFocus())
})

test('starts a new contact task on today at normal priority', async () => {
	renderAt(`/contacts/${contactID}`)

	expect(await screen.findByLabelText('Due date')).toHaveValue(today)
	expect(screen.getByLabelText('Priority')).toHaveTextContent('Normal')
	expect(screen.getByText('Priority', { selector: 'label' })).toBeInTheDocument()
})

test('adds a task for the contact at the chosen priority', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Send the invoice')
	await userEvent.click(screen.getByLabelText('Priority'))
	await userEvent.click(await screen.findByRole('option', { name: 'High' }))
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toMatchObject({ title: 'Send the invoice', contact_id: contactID, due_on: today, priority: 1 })
	const row = await screen.findByRole('listitem', { name: 'Send the invoice' })
	expect(within(row).getByText('High')).toBeInTheDocument()
})

test('adds a task for the contact due on the chosen day', async () => {
	const chosen = localDate(3)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Send the invoice')
	const due = screen.getByLabelText('Due date')
	await userEvent.clear(due)
	await userEvent.type(due, chosen)
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toMatchObject({ title: 'Send the invoice', contact_id: contactID, due_on: chosen })
	const row = await screen.findByRole('listitem', { name: 'Send the invoice' })
	expect(within(row).getByText(dueLabel(chosen))).toBeInTheDocument()
	expect(screen.getByText('Task added.')).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'View' })).not.toBeInTheDocument()
})

test('keeps Add task off while the due date is empty', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Send the invoice')
	await userEvent.clear(screen.getByLabelText('Due date'))

	expect(screen.getByRole('button', { name: 'Add task' })).toHaveAttribute('aria-disabled', 'true')
})

test('adds a task for the contact at the chosen time and shows the time on its row', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Send the invoice')
	await userEvent.type(screen.getByLabelText('Time'), '16:45')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toMatchObject({
		title: 'Send the invoice',
		contact_id: contactID,
		due_on: today,
		due_time: '16:45',
	})
	const row = await screen.findByRole('listitem', { name: 'Send the invoice' })
	expect(within(row).getByText('16:45')).toHaveClass('alphone-tasks__time')
	await waitFor(() => expect(screen.getByLabelText('Time')).toHaveValue(''))
})

test('adds a task for the contact with no time while the Time field stays empty', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Send the invoice')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).not.toHaveProperty('due_time')
})

test('lays the task title, the due date, the time, the priority and Add task on one form row', async () => {
	server.use(
		graphql.mutation('CreateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })
	const title = screen.getByRole('textbox', { name: 'Task title' })
	const due = screen.getByLabelText('Due date')
	const time = screen.getByLabelText('Time')
	const priority = screen.getByLabelText('Priority')
	const add = screen.getByRole('button', { name: 'Add task' })

	const row = title.closest('.godmin-form__row')
	expect(row).not.toBeNull()
	expect(row?.parentElement).toHaveClass('godmin-form')
	const cells = [...(row as Element).children]
	expect(cells).toHaveLength(5)
	const fieldCells = [title, due, time, priority].map((field) => cells.find((cell) => cell.contains(field)))
	expect(fieldCells).not.toContain(undefined)
	expect(new Set(fieldCells).size).toBe(4)
	expect(fieldCells.map((cell) => cells.indexOf(cell as Element))).toEqual([0, 1, 2, 3])
	expect(add.parentElement).toBe(row)
	expect(cells.at(-1)).toBe(add)

	await userEvent.type(title, 'X')
	await userEvent.click(add)

	const notice = await screen.findByText('The task could not be added.')
	expect(notice.closest('.godmin-form__row')).toBeNull()
})

test('lets the contact task form fill its column as one row', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	const form = screen.getByRole('textbox', { name: 'Task title' }).closest('form')
	expect(form).toHaveClass('godmin-form', 'godmin-form--inline', 'alphone-tasks__add', 'alphone-tasks__add--contact')
})

test('gives the contact task title the widest share of the form row', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })
	const title = screen.getByRole('textbox', { name: 'Task title' })
	const row = title.closest('.godmin-form__row') as Element

	expect(row.querySelectorAll('.godmin-form__grow')).toHaveLength(1)
	const grown = title.closest('.godmin-form__grow')
	expect(grown).not.toBeNull()
	expect(grown?.parentElement).toBe(row)
	expect(screen.getByLabelText('Due date').closest('.godmin-form__grow')).toBeNull()
	expect(screen.getByLabelText('Time').closest('.godmin-form__grow')).toBeNull()
})

test('names the contact task title with a visible label and no placeholder', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	const label = screen.getByText('Task title', { selector: 'label' })
	expect(label.closest('[data-visually-hidden]')).toBeNull()
	expect(screen.getByRole('textbox', { name: 'Task title' })).not.toHaveAttribute('placeholder')
})

test('starts the next task on today at normal priority after an add', async () => {
	renderAt(`/contacts/${contactID}`)
	await screen.findByRole('list', { name: 'Contact tasks' })

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Send the invoice')
	const due = screen.getByLabelText('Due date')
	await userEvent.clear(due)
	await userEvent.type(due, localDate(3))
	await userEvent.click(screen.getByLabelText('Priority'))
	await userEvent.click(await screen.findByRole('option', { name: 'High' }))
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	await waitFor(() => expect(screen.getByLabelText('Due date')).toHaveValue(today))
	expect(screen.getByLabelText('Priority')).toHaveTextContent('Normal')
	expect(screen.getByRole('textbox', { name: 'Task title' })).toHaveValue('')
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

test('shows a high task added after loading more at the top of the contact tasks', async () => {
	tasks = Array.from({ length: 51 }, (_, at) =>
		taskRow(`0198c000-0000-7000-8000-${String(2000 + at).padStart(12, '0')}`, `Routine task ${at + 1}`, today),
	)
	server.use(
		graphql.query('ContactDetail', ({ variables }) => {
			const listed = tasks.toSorted(serverOrder)
			const start = variables.after ? listed.findIndex((row) => row.id === variables.after) + 1 : 0
			const end = start + Number(variables.first)
			const shown = listed.slice(start, end)
			const endCursor = end < listed.length ? (shown.at(-1)?.id ?? null) : null
			return HttpResponse.json({ data: { contact: contactDetail(String(variables.id), shown, endCursor) } })
		}),
	)
	renderAt(`/contacts/${contactID}`)
	await screen.findByText('Routine task 1')
	await userEvent.click(screen.getByRole('button', { name: 'Load more tasks' }))
	await screen.findByText('Routine task 51')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Approve the pricing')
	await userEvent.click(screen.getByLabelText('Priority'))
	await userEvent.click(await screen.findByRole('option', { name: 'High' }))
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	const listed = screen.getByRole('list', { name: 'Contact tasks' })
	await waitFor(() => expect(within(listed).getAllByRole('listitem')).toHaveLength(52))
	expect(within(listed).getAllByRole('listitem')[0]).toHaveAttribute('aria-label', 'Approve the pricing')
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

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'X')
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

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('The task could not be added.')).toBeInTheDocument()
	expect(screen.queryByText('Task added.')).not.toBeInTheDocument()
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
	await userEvent.click(within(row).getByRole('button', { name: 'Postpone' }))

	expect(await screen.findByText('The task could not be updated.')).toBeInTheDocument()
	expect(screen.queryByText(movedToast(tomorrow))).not.toBeInTheDocument()
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

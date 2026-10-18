// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberFormatLocale } from '@alphone/frontend-sdk'
import { HttpResponse, graphql, http, server, textClasses } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { configureAppErrorText } from '../i18n/errors'
import { busyClasses, buttonClasses, compactClasses, liveStream, renderAt } from './render'

const callID = '0198c000-0000-7000-8000-000000000101'
const quoteID = '0198c000-0000-7000-8000-000000000102'
const doneID = '0198c000-0000-7000-8000-000000000103'
const addedID = '0198c000-0000-7000-8000-000000000104'
const oldID = '0198c000-0000-7000-8000-000000000105'
const refileID = '0198c000-0000-7000-8000-000000000106'
const contactID = '0198c000-0000-7000-8000-000000000107'
const linkedContact = { id: contactID, name: 'Maria Perez' }

// These dates are computed independently of the screen's own date helpers.
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

/**
 * Returns the toast a quick add for a day other than the one shown raises.
 * @param iso - The day the task was added for as YYYY-MM-DD.
 * @returns The toast text.
 */
function addedToast(iso: string) {
	return `Task added for ${iso.split('-').reverse().join('/')}.`
}

const today = localDate(0)
const tomorrow = localDate(1)
const yesterday = localDate(-1)

function taskRow(id: string, title: string, status = 'open', priority = 0) {
	return {
		id,
		title,
		status,
		priority,
		due_on: today,
		due_time: null as string | null,
		contact: null as typeof linkedContact | null,
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

function taskPage(rows: ReturnType<typeof taskRow>[], endCursor: string | null = null) {
	return {
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
				contact: row.contact === null ? null : { __typename: 'Contact', ...row.contact },
			},
			cursor: row.id,
		})),
		pageInfo: { __typename: 'PageInfo', hasNextPage: endCursor !== null, endCursor },
	}
}

let listedDates: string[] = []
let overdueBefore: string[] = []
let patched: { id: string; body: Record<string, unknown> }[] = []
let created: Record<string, unknown>[] = []
let tasks: ReturnType<typeof taskRow>[] = []
let overdue: ReturnType<typeof taskRow>[] = []

beforeEach(() => {
	listedDates = []
	overdueBefore = []
	patched = []
	created = []
	overdue = []
	tasks = [
		taskRow(callID, 'Call the supplier'),
		taskRow(quoteID, 'Send the quote'),
		taskRow(doneID, 'Book the courier', 'done'),
	]
	server.use(
		graphql.query('DayTasks', ({ variables }) => {
			listedDates.push(String(variables.date))
			const status = String(variables.status)
			return HttpResponse.json({
				data: { tasks: taskPage(tasks.filter((row) => row.status === status)) },
			})
		}),
		graphql.query('OverdueTasks', ({ variables }) => {
			const dueBefore = String(variables.dueBefore)
			overdueBefore.push(dueBefore)
			return HttpResponse.json({ data: { tasks: taskPage(overdue.filter((row) => row.due_on < dueBefore)) } })
		}),
		graphql.mutation('UpdateTask', ({ variables }) => {
			const input = variables.input as { status?: string; dueOn?: string; dueTime?: string | null }
			const id = String(variables.id)
			const timed = 'dueTime' in input ? { due_time: input.dueTime } : {}
			patched.push({ id, body: { status: input.status, due_on: input.dueOn, ...timed } })
			const stored = [...tasks, ...overdue].find((row) => row.id === id)
			if (stored === undefined) {
				return HttpResponse.json({ data: null, errors: [{ message: 'task: not found' }] })
			}
			const updated = {
				...stored,
				status: input.status ?? stored.status,
				due_on: input.dueOn ?? stored.due_on,
				due_time: input.dueTime === undefined ? stored.due_time : input.dueTime,
			}
			tasks = tasks.map((row) => (row.id === updated.id ? updated : row))
			overdue = overdue.map((row) => (row.id === updated.id ? updated : row))
			return HttpResponse.json({ data: { updateTask: taskNode(updated) } })
		}),
		graphql.mutation('CreateTask', ({ variables }) => {
			const input = variables.input as { title: string; dueOn: string; dueTime?: string; priority?: number }
			created.push({ ...input })
			const row = {
				...taskRow(addedID, input.title, 'open', input.priority ?? 0),
				due_on: input.dueOn,
				due_time: input.dueTime ?? null,
			}
			tasks = [...tasks, row]
			return HttpResponse.json({
				data: { createTask: { __typename: 'CreateTaskPayload', task: taskNode(row), replay: false } },
			})
		}),
	)
})

/** Serves the open and done lists of each day from the tasks due on it. */
function servePerDay() {
	server.use(
		graphql.query('DayTasks', ({ variables }) => {
			const date = String(variables.date)
			const status = String(variables.status)
			return HttpResponse.json({
				data: { tasks: taskPage(tasks.filter((row) => row.due_on === date && row.status === status)) },
			})
		}),
	)
}

/** Picks a day in the quick add due date field. */
async function pickDue(day: string) {
	const due = screen.getByLabelText('Due date')
	await userEvent.clear(due)
	await userEvent.type(due, day)
}

/** Types a time in the quick add time field. */
async function pickTime(time: string) {
	await userEvent.type(screen.getByLabelText('Time'), time)
}

/** Picks the High priority in the quick add priority field. */
async function pickHigh() {
	await userEvent.click(screen.getByLabelText('Priority'))
	await userEvent.click(await screen.findByRole('option', { name: 'High' }))
}

test('shows the add button busy and still refuses a second submit', async () => {
	const busy = busyClasses()
	expect(busy.length).toBeGreaterThan(0)
	server.use(graphql.mutation('CreateTask', () => new Promise(() => {})))
	renderAt('/tasks')
	await screen.findByRole('heading', { level: 1, name: 'Tasks' })
	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Call Maria Perez')

	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	const add = screen.getByRole('button', { name: 'Add task' })
	await waitFor(() => {
		for (const token of busy) {
			expect([...add.classList]).toContain(token)
		}
	})
	expect(add).toHaveAttribute('aria-disabled', 'true')
})

test('ghosts the rows while the day of tasks arrives', async () => {
	server.use(graphql.query('DayTasks', () => new Promise(() => {})))
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

test('draws every header button compact, as a WordPress page header does', async () => {
	renderAt(`/tasks?date=${tomorrow}`)

	const add = await screen.findByRole('link', { name: 'New task' })
	expect([...add.classList]).toEqual(buttonClasses('solid', 'compact'))
	for (const name of ['Previous day', 'Today', 'Next day']) {
		expect([...screen.getByRole('button', { name }).classList]).toEqual(expect.arrayContaining(compactClasses()))
	}
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
	expect(screen.getByText('Task completed.')).toBeInTheDocument()
})

test('undoes a completion from its toast', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))
	await screen.findByText('Task completed.')

	await userEvent.click(screen.getByRole('button', { name: 'Undo' }))

	await waitFor(() => expect(patched).toHaveLength(2))
	expect(patched[1]).toEqual({ id: callID, body: { status: 'open', due_on: undefined } })
	await waitFor(() => {
		const open = screen.getByRole('list', { name: 'Open tasks' })
		expect(within(open).getByText('Call the supplier')).toBeInTheDocument()
	})
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
	expect(screen.getByText('Task reopened.')).toBeInTheDocument()
})

test('undoes a reopen from its toast', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	await userEvent.click(screen.getByRole('button', { name: 'Done (1)' }))
	const row = await screen.findByRole('listitem', { name: 'Book the courier' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Reopen' }))
	await screen.findByText('Task reopened.')

	await userEvent.click(screen.getByRole('button', { name: 'Undo' }))

	await waitFor(() => expect(patched).toHaveLength(2))
	expect(patched[1]).toEqual({ id: doneID, body: { status: 'done', due_on: undefined } })
})

test('shows a failed undo in the notice of the screen', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))
	await screen.findByText('Task completed.')
	server.use(
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	await userEvent.click(screen.getByRole('button', { name: 'Undo' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('The task could not be updated.')
})

test('marks a raised priority on the row', async () => {
	tasks = [taskRow(callID, 'Call the supplier', 'open', 1)]

	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })
	expect(within(row).getByText('High')).toBeInTheDocument()
})

test('shows the time of a timed task in its own column before the title', async () => {
	tasks = [{ ...taskRow(callID, 'Call the supplier'), due_time: '09:30' }]

	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })
	const time = within(row).getByText('09:30')
	expect(time.tagName).toBe('TIME')
	expect(time).toHaveAttribute('datetime', '09:30')
	expect(time).toHaveClass('alphone-tasks__time')
	expect([...time.classList]).toEqual(expect.arrayContaining(textClasses('body-md')))
	expect(row.querySelector('.alphone-tasks__check')?.compareDocumentPosition(time)).toBe(
		Node.DOCUMENT_POSITION_FOLLOWING,
	)
	expect(time.nextElementSibling).toHaveClass('alphone-tasks__title')
})

test('leaves the time column blank on a task with no time', async () => {
	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })
	const column = row.querySelector('.alphone-tasks__time')
	expect(column).not.toBeNull()
	expect(column).toBeEmptyDOMElement()
	expect(column?.tagName).not.toBe('TIME')
	expect(row.querySelector('.alphone-tasks__check')?.compareDocumentPosition(column as Element)).toBe(
		Node.DOCUMENT_POSITION_FOLLOWING,
	)
	expect(column?.nextElementSibling).toHaveClass('alphone-tasks__title')
})

test('holds the time and the title in one lead group after the check, on timed and untimed rows', async () => {
	tasks = [{ ...taskRow(callID, 'Call the supplier'), due_time: '09:30' }, taskRow(quoteID, 'Send the quote')]

	renderAt('/tasks')

	const list = await screen.findByRole('list', { name: 'Open tasks' })
	await within(list).findByRole('listitem', { name: 'Send the quote' })
	for (const row of within(list).getAllByRole('listitem')) {
		const check = row.querySelector('.alphone-tasks__check')
		const lead = row.querySelector('.alphone-tasks__time')?.parentElement
		expect(lead).toHaveClass('alphone-tasks__lead')
		expect(lead?.parentElement).toBe(row)
		expect(check?.parentElement).toBe(row)
		expect(check?.compareDocumentPosition(lead as Element)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
		expect([...(lead?.children ?? [])]).toEqual([
			row.querySelector('.alphone-tasks__time'),
			row.querySelector('.alphone-tasks__title'),
		])
		expect(lead).toHaveStyle({ alignItems: 'start' })
	}
})

test('writes the time in the format locale', async () => {
	rememberFormatLocale('en-US')
	tasks = [{ ...taskRow(callID, 'Call the supplier'), due_time: '21:05' }]

	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })
	expect(within(row).getByText('09:05 PM')).toHaveAttribute('datetime', '21:05')
})

test('sizes the time column alike on timed and untimed rows from the widest times of the format locale', async () => {
	rememberFormatLocale('en-US')
	tasks = [{ ...taskRow(callID, 'Call the supplier'), due_time: '09:30' }, taskRow(quoteID, 'Send the quote')]

	renderAt('/tasks')

	await screen.findByRole('listitem', { name: 'Call the supplier' })
	const timed = screen.getByRole('listitem', { name: 'Call the supplier' }).querySelector('.alphone-tasks__time')
	const untimed = screen.getByRole('listitem', { name: 'Send the quote' }).querySelector('.alphone-tasks__time')
	expect(timed).toHaveTextContent('09:30 AM')
	for (const column of [timed, untimed]) {
		expect(column).toHaveAttribute('data-morning', '10:00 AM')
		expect(column).toHaveAttribute('data-evening', '10:00 PM')
		expect(column?.className).toBe(timed?.className)
	}
})

test('shows the time before the title on done work too', async () => {
	tasks = [taskRow(callID, 'Call the supplier'), { ...taskRow(doneID, 'Book the courier', 'done'), due_time: '08:15' }]
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.click(screen.getByRole('button', { name: 'Done (1)' }))

	const row = await screen.findByRole('listitem', { name: 'Book the courier' })
	const time = within(row).getByText('08:15')
	expect(time).toHaveClass('alphone-tasks__time')
	expect(time.nextElementSibling).toHaveClass('alphone-tasks__title')
})

test('shows the time before the title on overdue work and keeps its due date after it', async () => {
	overdue = [{ ...taskRow(oldID, 'Chase the invoice'), due_on: yesterday, due_time: '11:00' }]

	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Chase the invoice' })
	const time = within(row).getByText('11:00')
	const title = row.querySelector('.alphone-tasks__title') as Element
	const due = within(row).getByText(dueLabel(yesterday))
	expect(time.nextElementSibling).toBe(title)
	expect(title.compareDocumentPosition(due) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
	expect(due).toHaveClass('alphone-tasks__due')
})

test('keeps the time of a postponed task, sending the new day alone', async () => {
	servePerDay()
	tasks = [{ ...taskRow(callID, 'Call the supplier'), due_time: '09:30' }]
	renderAt('/tasks')
	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })

	await userEvent.click(within(row).getByRole('button', { name: 'Postpone' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toEqual({ id: callID, body: { due_on: tomorrow } })
	await screen.findByText(movedToast(tomorrow))
	await userEvent.click(screen.getByRole('button', { name: 'Next day' }))
	const moved = await screen.findByRole('listitem', { name: 'Call the supplier' })
	expect(within(moved).getByText('09:30')).toHaveClass('alphone-tasks__time')
})

test('opens the linked contact from its name after the task title', async () => {
	tasks = [{ ...taskRow(callID, 'Call the supplier'), contact: linkedContact }]
	server.use(
		graphql.query('ContactDetail', () =>
			HttpResponse.json({
				data: {
					contact: {
						__typename: 'Contact',
						...linkedContact,
						createdAt: '2026-07-06T10:00:00Z',
						identities: [],
						tasks: taskPage([]),
					},
				},
			}),
		),
	)
	renderAt('/tasks')
	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })

	const open = within(row).getByRole('link', { name: 'Maria Perez' })
	expect(open).toHaveAttribute('href', `/contacts/${contactID}`)
	await userEvent.click(open)

	expect(await screen.findByRole('heading', { level: 1, name: 'Maria Perez' })).toBeInTheDocument()
})

test('shows the contact name only on a task linked to a contact', async () => {
	tasks = [{ ...taskRow(callID, 'Call the supplier'), contact: linkedContact }, taskRow(quoteID, 'Send the quote')]

	renderAt('/tasks')

	const linked = await screen.findByRole('listitem', { name: 'Call the supplier' })
	expect(within(linked).getByRole('link', { name: 'Maria Perez' })).toBeInTheDocument()
	const unlinked = screen.getByRole('listitem', { name: 'Send the quote' })
	expect(within(unlinked).getAllByRole('link')).toHaveLength(1)
	expect(within(unlinked).getByRole('link', { name: 'Send the quote' })).toBeInTheDocument()
})

test('sets the contact name right after the task title, as a small quiet button with the person icon', async () => {
	const small = buttonClasses('solid', 'small').filter((token) => !buttonClasses('solid').includes(token))
	const quiet = buttonClasses('minimal', 'small').filter((token) => !buttonClasses('solid', 'small').includes(token))
	tasks = [{ ...taskRow(callID, 'Call the supplier'), contact: linkedContact }]

	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })
	const title = within(row).getByRole('link', { name: 'Call the supplier' })
	const open = within(row).getByRole('link', { name: 'Maria Perez' })
	const cell = row.querySelector('.alphone-tasks__title')
	expect(cell).toContainElement(title)
	expect(cell).toContainElement(open)
	expect(title).toHaveClass('alphone-tasks__title-link')
	expect(title).not.toContainElement(open)
	expect(title.compareDocumentPosition(open) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
	expect(small.length).toBeGreaterThan(0)
	expect(quiet.length).toBeGreaterThan(0)
	expect([...open.classList]).toEqual(expect.arrayContaining([...small, ...quiet]))
	expect(open).toHaveClass('alphone-tasks__open-contact')
	expect(open.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
	expect(open.querySelector('svg')).toHaveClass('alphone-tasks__contact-icon')
	expect(open.querySelector('svg')?.compareDocumentPosition(within(open).getByText('Maria Perez'))).toBe(
		Node.DOCUMENT_POSITION_FOLLOWING,
	)
})

test('writes the contact name in a part that keeps a long name on one line', async () => {
	tasks = [{ ...taskRow(callID, 'Call the supplier'), contact: linkedContact }]

	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })
	const open = within(row).getByRole('link', { name: 'Maria Perez' })
	expect(within(open).getByText('Maria Perez')).toHaveClass('alphone-tasks__contact-name')
})

test('shows the contact name on overdue work linked to a contact', async () => {
	overdue = [{ ...taskRow(oldID, 'Chase the invoice'), due_on: yesterday, contact: linkedContact }]

	renderAt('/tasks')

	const row = await screen.findByRole('listitem', { name: 'Chase the invoice' })
	expect(within(row).getByRole('link', { name: 'Maria Perez' })).toHaveAttribute(
		'href',
		`/contacts/${contactID}`,
	)
})

test('adds a task from the quick add field', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(
		screen.getByRole('textbox', { name: 'Task title' }),
		'Order more boxes',
	)
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('Order more boxes')).toBeInTheDocument()
	expect(screen.getByRole('textbox', { name: 'Task title' })).toHaveValue('')
	expect(screen.getByText('Task added.')).toBeInTheDocument()
})

test('starts the quick add on the day shown at normal priority', async () => {
	renderAt(`/tasks?date=${tomorrow}`)
	await screen.findByText('Call the supplier')

	expect(screen.getByLabelText('Due date')).toHaveValue(tomorrow)
	expect(screen.getByLabelText('Priority')).toHaveTextContent('Normal')
})

test('names the quick add fields as the New task screen does', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	expect(screen.getByText('Due date', { selector: 'label' })).toBeInTheDocument()
	expect(screen.getByText('Time', { selector: 'label' })).toBeInTheDocument()
	expect(screen.getByText('Priority', { selector: 'label' })).toBeInTheDocument()
})

test('starts the quick add with no time', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	expect(screen.getByLabelText('Time')).toHaveValue('')
	expect(screen.getByLabelText('Time')).toHaveAttribute('type', 'time')
})

test('adds a task at the chosen time from the quick add field and shows the time on its row', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await pickTime('09:30')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toEqual({ title: 'Order more boxes', dueOn: today, dueTime: '09:30', priority: 0 })
	const row = await screen.findByRole('listitem', { name: 'Order more boxes' })
	expect(within(row).getByText('09:30')).toHaveClass('alphone-tasks__time')
	expect(screen.getByText('Task added.')).toBeInTheDocument()
})

test('starts the next quick add with no time after an add', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await pickTime('09:30')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	await waitFor(() => expect(screen.getByLabelText('Time')).toHaveValue(''))
})

test('adds a task with no time once the reader empties the quick add time', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await pickTime('09:30')
	await userEvent.clear(screen.getByLabelText('Time'))
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toEqual({ title: 'Order more boxes', dueOn: today, priority: 0 })
})

test('adds a task on the day shown at normal priority', async () => {
	renderAt(`/tasks?date=${tomorrow}`)
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toEqual({ title: 'Order more boxes', dueOn: tomorrow, priority: 0 })
	expect(await screen.findByText('Task added.')).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'View' })).not.toBeInTheDocument()
})

test('adds a task on the chosen day at the chosen priority', async () => {
	servePerDay()
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await pickDue(today)
	await pickHigh()
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	expect(created[0]).toEqual({ title: 'Order more boxes', dueOn: today, priority: 1 })
	const row = await screen.findByRole('listitem', { name: 'Order more boxes' })
	expect(within(row).getByText('High')).toBeInTheDocument()
})

test('names the day a task was added for, keeps it out of the day shown and opens its day from the toast', async () => {
	servePerDay()
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	await userEvent.click(screen.getByRole('button', { name: 'Next day' }))
	await screen.findByText('Nothing due today.')
	await userEvent.click(await screen.findByRole('button', { name: 'Today' }))
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await pickDue(tomorrow)
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText(addedToast(tomorrow))).toBeInTheDocument()
	expect(screen.queryByText('Task added.')).not.toBeInTheDocument()
	const open = screen.getByRole('list', { name: 'Open tasks' })
	expect(within(open).queryByText('Order more boxes')).not.toBeInTheDocument()

	await userEvent.click(screen.getByRole('button', { name: 'View' }))

	expect(await screen.findByRole('listitem', { name: 'Order more boxes' })).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Today' })).toBeInTheDocument()
})

test('returns focus to the task title after an add', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	const title = screen.getByRole('textbox', { name: 'Task title' })

	await userEvent.type(title, 'Order more boxes')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	await waitFor(() => expect(title).toHaveFocus())
})

test('confirms an add that lands after the screen was left', async () => {
	let release: (() => void) | undefined
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		graphql.mutation('CreateTask', async ({ variables }) => {
			await held
			const input = variables.input as { title: string; dueOn: string }
			const row = { ...taskRow(addedID, input.title), due_on: input.dueOn }
			return HttpResponse.json({
				data: { createTask: { __typename: 'CreateTaskPayload', task: taskNode(row), replay: false } },
			})
		}),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))
	await userEvent.click(screen.getByRole('link', { name: 'New task' }))
	await screen.findByRole('heading', { level: 1, name: 'New task' })
	release?.()

	expect(await screen.findByText('Task added.')).toBeInTheDocument()
})

test('names the day of an add that lands after the day shown changed', async () => {
	servePerDay()
	let release: (() => void) | undefined
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		graphql.mutation('CreateTask', async ({ variables }) => {
			await held
			const input = variables.input as { title: string; dueOn: string }
			const row = { ...taskRow(addedID, input.title), due_on: input.dueOn }
			return HttpResponse.json({
				data: { createTask: { __typename: 'CreateTaskPayload', task: taskNode(row), replay: false } },
			})
		}),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))
	await userEvent.click(screen.getByRole('button', { name: 'Next day' }))
	await screen.findByText('Nothing due today.')
	release?.()

	expect(await screen.findByText(addedToast(today))).toBeInTheDocument()
	expect(screen.queryByText('Task added.')).not.toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'View' })).toBeInTheDocument()
})

test('starts the next task on the day shown at normal priority after an add', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await pickDue(localDate(3))
	await pickHigh()
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(created).toHaveLength(1))
	await waitFor(() => expect(screen.getByRole('textbox', { name: 'Task title' })).toHaveValue(''))
	expect(screen.getByLabelText('Due date')).toHaveValue(today)
	expect(screen.getByLabelText('Priority')).toHaveTextContent('Normal')
})

test('moves the quick add due date along with the day shown and keeps a typed title', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await pickDue(localDate(3))

	await userEvent.click(screen.getByRole('button', { name: 'Next day' }))

	await waitFor(() => expect(screen.getByLabelText('Due date')).toHaveValue(tomorrow))
	expect(screen.getByRole('textbox', { name: 'Task title' })).toHaveValue('Order more boxes')
})

test('keeps Add task off while the quick add due date is empty', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Order more boxes')
	await userEvent.clear(screen.getByLabelText('Due date'))

	expect(screen.getByRole('button', { name: 'Add task' })).toHaveAttribute('aria-disabled', 'true')
})

test('does not add a task without a title', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	expect(screen.getByRole('button', { name: 'Add task' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('lays the task title, the due date, the time, the priority and Add task on one form row', async () => {
	server.use(
		graphql.mutation('CreateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
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
	expect(fieldCells).not.toContain(add)
	expect(add.parentElement).toBe(row)
	expect(cells.at(-1)).toBe(add)

	await userEvent.type(title, 'X')
	await userEvent.click(add)

	const notice = await screen.findByText('The task could not be added.')
	expect(notice.closest('.godmin-form__row')).toBeNull()
})

test('lets the quick add form fill its column as one row', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const form = screen.getByRole('textbox', { name: 'Task title' }).closest('form')
	expect(form).toHaveClass('godmin-form', 'godmin-form--inline', 'alphone-tasks__add')
})

test('gives the task title the widest share of the form row', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')
	const title = screen.getByRole('textbox', { name: 'Task title' })
	const row = title.closest('.godmin-form__row') as Element

	expect(row.querySelectorAll('.godmin-form__grow')).toHaveLength(1)
	const grown = title.closest('.godmin-form__grow')
	expect(grown).not.toBeNull()
	expect(grown?.parentElement).toBe(row)
})

test('names the task title with a visible label and no placeholder', async () => {
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const label = screen.getByText('Task title', { selector: 'label' })
	expect(label.closest('[data-visually-hidden]')).toBeNull()
	expect(screen.getByRole('textbox', { name: 'Task title' })).not.toHaveAttribute('placeholder')
})

test('shows an empty state when the day has no tasks', async () => {
	server.use(
		graphql.query('DayTasks', () => HttpResponse.json({ data: { tasks: taskPage([]) } })),
	)

	renderAt('/tasks')

	expect(await screen.findByText('Nothing due today.')).toBeInTheDocument()
})

test('loads more tasks through the cursor', async () => {
	server.use(
		graphql.query('DayTasks', ({ variables }) =>
			HttpResponse.json({
				data: {
					tasks: variables.after
						? taskPage([taskRow(quoteID, 'Send the quote')])
						: taskPage([taskRow(callID, 'Call the supplier')], 'CUR1'),
				},
			}),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.click(screen.getByRole('button', { name: 'Load more' }))

	expect(await screen.findByText('Send the quote')).toBeInTheDocument()
})

/**
 * Orders two open tasks of a day as the server lists them, by priority, time with an untimed one after, then creation.
 * @param first - One task.
 * @param second - The other task.
 * @returns A negative number when the first task lists first.
 */
function serverOrder(first: ReturnType<typeof taskRow>, second: ReturnType<typeof taskRow>) {
	const clock = (row: ReturnType<typeof taskRow>) => row.due_time ?? '24:00'
	return (
		second.priority - first.priority ||
		clock(first).localeCompare(clock(second)) ||
		first.id.localeCompare(second.id)
	)
}

/** Serves each day list a page at a time after the cursor it names, in the order the server lists it. */
function serveRanked() {
	server.use(
		graphql.query('DayTasks', ({ variables }) => {
			const listed = tasks
				.filter((row) => row.status === String(variables.status))
				.toSorted(serverOrder)
			const start = variables.after ? listed.findIndex((row) => row.id === variables.after) + 1 : 0
			const end = start + Number(variables.first)
			const shown = listed.slice(start, end)
			return HttpResponse.json({
				data: { tasks: taskPage(shown, end < listed.length ? (shown.at(-1)?.id ?? null) : null) },
			})
		}),
	)
}

/**
 * Lists the given number of open routine tasks due today.
 * @param count - How many tasks to list.
 * @returns The task rows.
 */
function routineTasks(count: number) {
	return Array.from({ length: count }, (_, at) =>
		taskRow(`0198c000-0000-7000-8000-${String(1000 + at).padStart(12, '0')}`, `Routine task ${at + 1}`),
	)
}

test('shows a high task added after loading more at the top of the day', async () => {
	tasks = routineTasks(51)
	serveRanked()
	renderAt('/tasks')
	await screen.findByText('Routine task 1')
	await userEvent.click(screen.getByRole('button', { name: 'Load more' }))
	await screen.findByText('Routine task 51')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'Approve the pricing')
	await pickHigh()
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	const open = screen.getByRole('list', { name: 'Open tasks' })
	await waitFor(() => expect(within(open).getAllByRole('listitem')).toHaveLength(52))
	expect(within(open).getAllByRole('listitem')[0]).toHaveAttribute('aria-label', 'Approve the pricing')
})

test('keeps the day in the order the server lists it, each row with its time', async () => {
	tasks = [
		taskRow(callID, 'Call the supplier'),
		{ ...taskRow(quoteID, 'Send the quote', 'open', 1), due_time: '11:00' },
		{ ...taskRow(oldID, 'Chase the invoice'), due_time: '09:00' },
		{ ...taskRow(refileID, 'Refile the paperwork', 'open', 1), due_time: '09:00' },
	]
	serveRanked()

	renderAt('/tasks')

	await screen.findByText('Call the supplier')
	const rows = within(screen.getByRole('list', { name: 'Open tasks' })).getAllByRole('listitem')
	expect(rows.map((row) => row.getAttribute('aria-label'))).toEqual([
		'Refile the paperwork',
		'Send the quote',
		'Chase the invoice',
		'Call the supplier',
	])
	const times = rows.map((row) => row.querySelector('.alphone-tasks__time')?.textContent)
	expect(times).toEqual(['09:00', '11:00', '09:00', ''])
})

test('shows open work even when finished tasks fill the first page, counting them in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	const finished = [
		taskRow(doneID, 'Book the courier', 'done'),
		taskRow(oldID, 'File the customs form', 'done'),
	]
	server.use(
		graphql.query('DayTasks', ({ variables }) =>
			HttpResponse.json({
				data: {
					tasks: variables.status === 'open'
						? taskPage([taskRow(callID, 'Reply to the imported enquiry')])
						: taskPage(finished),
				},
			}),
		),
	)

	renderAt('/tasks')

	expect(await screen.findByText('Reply to the imported enquiry')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Done (२)' })).toBeInTheDocument()
})

test('keeps the day usable when the done group cannot be loaded', async () => {
	server.use(
		graphql.query('DayTasks', ({ variables }) =>
			variables.status === 'done'
				? HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })
				: HttpResponse.json({
					data: { tasks: taskPage([taskRow(callID, 'Call the supplier')]) },
				}),
		),
	)

	renderAt('/tasks')

	expect(await screen.findByText('Call the supplier')).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: /^Done \(/ })).not.toBeInTheDocument()
})

test('loads more done tasks through their own cursor', async () => {
	server.use(
		graphql.query('DayTasks', ({ variables }) => {
			if (variables.status !== 'done') {
				return HttpResponse.json({ data: { tasks: taskPage([]) } })
			}
			return HttpResponse.json({
				data: {
					tasks: variables.after
						? taskPage([taskRow(oldID, 'File the customs form', 'done')])
						: taskPage([taskRow(doneID, 'Book the courier', 'done')], 'D1'),
				},
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
	const live = liveStream()
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	tasks = [...tasks, taskRow(addedID, 'Created by an automation')]
	await live.announce('task.created')

	expect(await screen.findByText('Created by an automation')).toBeInTheDocument()
})

test('reports when the tasks cannot be loaded', async () => {
	server.use(
		graphql.query('DayTasks', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	renderAt('/tasks')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Tasks could not be loaded.',
	)
})

test('reports when a task cannot be added', async () => {
	server.use(
		graphql.mutation('CreateTask', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'task: empty title', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('task: empty title')).toBeInTheDocument()
})

test('reports a generic message when adding fails otherwise', async () => {
	server.use(
		graphql.mutation('CreateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText('The task could not be added.')).toBeInTheDocument()
	expect(screen.queryByText('Task added.')).not.toBeInTheDocument()
})

test('tells the reader why the add was refused as a write from another site', async () => {
	configureAppErrorText()
	server.use(
		http.post('/api/graphql', async ({ request }) => {
			const { operationName } = (await request.clone().json()) as { operationName?: string }
			return operationName === 'CreateTask'
				? HttpResponse.json({ error: 'cross-origin request refused', code: 'request_cross_origin' }, { status: 403 })
				: undefined
		}),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	expect(await screen.findByText(
		'This request came from a page on another site, so it was refused. Open the admin on this site and try again.',
	)).toBeInTheDocument()
	expect(screen.queryByText('The task could not be added.')).not.toBeInTheDocument()
})

test('reports when a task cannot be updated', async () => {
	server.use(
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

	expect(await screen.findByText('The task could not be updated.')).toBeInTheDocument()
	expect(screen.queryByText('Task completed.')).not.toBeInTheDocument()
})

test('reports the backend message when an update is rejected', async () => {
	server.use(
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'task: invalid status', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

	expect(await screen.findByText('The task could not be updated.')).toBeInTheDocument()
})



test('disables the row control while its update is in flight', async () => {
	let release: (() => void) | undefined
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		graphql.mutation('UpdateTask', async ({ variables }) => {
			await held
			const id = String(variables.id)
			const stored = tasks.find((row) => row.id === id) ?? taskRow(id, 'Call the supplier')
			return HttpResponse.json({
				data: { updateTask: taskNode({ ...stored, status: 'done' }) },
			})
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
		graphql.query('DayTasks', ({ variables }) =>
			HttpResponse.json({
				data: {
					tasks: variables.status === 'open'
						? taskPage([taskRow(callID, `Work due ${String(variables.date)}`)])
						: taskPage([]),
				},
			}),
		),
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

test('sets the overdue heading a size above the field labels', async () => {
	const large = textClasses('heading-lg')
	overdue = [{ ...taskRow(oldID, 'Chase the invoice'), due_on: yesterday }]

	renderAt('/tasks')

	const heading = await screen.findByRole('heading', { level: 2, name: 'Overdue' })
	expect([...heading.classList]).toEqual(expect.arrayContaining(large))
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
	await userEvent.click(within(row).getByRole('button', { name: 'Postpone' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toEqual({ id: callID, body: { due_on: tomorrow } })
	expect(await screen.findByText(movedToast(tomorrow))).toBeInTheDocument()
})

test('undoes a postpone from its toast with the day the task held', async () => {
	overdue = [{ ...taskRow(oldID, 'Chase the invoice'), due_on: localDate(-5) }]
	renderAt('/tasks')
	const row = await screen.findByRole('listitem', { name: 'Chase the invoice' })
	await userEvent.click(within(row).getByRole('button', { name: 'Postpone' }))
	await screen.findByText(movedToast(tomorrow))
	await waitFor(() => expect(screen.queryByRole('listitem', { name: 'Chase the invoice' })).not.toBeInTheDocument())

	await userEvent.click(screen.getByRole('button', { name: 'Undo' }))

	await waitFor(() => {
		expect(screen.queryByRole('alert')).not.toBeInTheDocument()
		expect(screen.getByRole('listitem', { name: 'Chase the invoice' })).toBeInTheDocument()
	})
	expect(patched[1]).toEqual({ id: oldID, body: { due_on: localDate(-5) } })
})

test('pushes an overdue task to tomorrow rather than to the day after it was due', async () => {
	overdue = [{ ...taskRow(oldID, 'Chase the invoice'), due_on: localDate(-5) }]
	renderAt('/tasks')
	const row = await screen.findByRole('listitem', { name: 'Chase the invoice' })

	await userEvent.click(within(row).getByRole('button', { name: 'Postpone' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toEqual({ id: oldID, body: { due_on: tomorrow } })
})

test('pushes a task on a future day to the day after it', async () => {
	tasks = [{ ...taskRow(callID, 'Call the supplier'), due_on: tomorrow }]
	renderAt(`/tasks?date=${tomorrow}`)
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('button', { name: 'Postpone' }))

	await waitFor(() => expect(patched).toHaveLength(1))
	expect(patched[0]).toEqual({ id: callID, body: { due_on: localDate(2) } })
})

test('loads more overdue work through the cursor', async () => {
	server.use(
		graphql.query('OverdueTasks', ({ variables }) =>
			HttpResponse.json({
				data: {
					tasks: variables.after
						? taskPage([{ ...taskRow(refileID, 'Refile the paperwork'), due_on: yesterday }])
						: taskPage([{ ...taskRow(oldID, 'Chase the invoice'), due_on: yesterday }], 'CUR1'),
				},
			}),
		),
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
	expect(within(row).queryByRole('button', { name: 'Postpone' })).not.toBeInTheDocument()
})

test('reports when overdue work cannot be loaded', async () => {
	server.use(
		graphql.query('OverdueTasks', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	renderAt('/tasks')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Overdue tasks could not be loaded.',
	)
})

test('drops the session when the overdue list is unauthorized', async () => {
	server.use(
		graphql.query('OverdueTasks', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)

	const client = renderAt('/tasks')

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('drops the session when the list is unauthorized', async () => {
	server.use(
		graphql.query('DayTasks', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)

	const client = renderAt('/tasks')

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('drops the session when adding is unauthorized', async () => {
	server.use(
		graphql.mutation('CreateTask', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt('/tasks')
	await screen.findByText('Call the supplier')

	await userEvent.type(screen.getByRole('textbox', { name: 'Task title' }), 'X')
	await userEvent.click(screen.getByRole('button', { name: 'Add task' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('drops the session when updating is unauthorized', async () => {
	server.use(
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('leaves the day alone when pushing a task fails', async () => {
	server.use(
		graphql.mutation('UpdateTask', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/tasks')
	await screen.findByText('Call the supplier')

	const row = screen.getByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('button', { name: 'Postpone' }))

	expect(await screen.findByText('The task could not be updated.')).toBeInTheDocument()
	expect(screen.getByText('Call the supplier')).toBeInTheDocument()
	expect(screen.queryByText(movedToast(tomorrow))).not.toBeInTheDocument()
})

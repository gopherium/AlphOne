// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberFormatLocale } from '@alphone/frontend-sdk'
import { textClasses } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { renderAt } from './render'
import {
	callID,
	dayTasks,
	doneID,
	dueLabel,
	movedToast,
	oldID,
	pickTime,
	quoteID,
	refileID,
	taskRow,
	taskServer,
	today,
	tomorrow,
	yesterday,
} from './tasksScreen'
import type { TaskRow } from './tasksScreen'

let tasks: TaskRow[] = []
let overdue: TaskRow[] = []
const { created, patched, serveDay, servePerDay, serveRanked } = taskServer(
	() => tasks,
	() => overdue,
)

beforeEach(() => {
	overdue = []
	tasks = dayTasks()
	serveDay()
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

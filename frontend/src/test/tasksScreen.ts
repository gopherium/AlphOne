// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

/** callID is the id of the first open task of the day. */
export const callID = '0198c000-0000-7000-8000-000000000101'
/** quoteID is the id of the second open task of the day. */
export const quoteID = '0198c000-0000-7000-8000-000000000102'
/** doneID is the id of the done task of the day. */
export const doneID = '0198c000-0000-7000-8000-000000000103'
/** addedID is the id the mocked server gives every task it creates. */
export const addedID = '0198c000-0000-7000-8000-000000000104'
/** oldID is the id of a task a test sets overdue or done. */
export const oldID = '0198c000-0000-7000-8000-000000000105'
/** refileID is the id of one more task a test adds to a list. */
export const refileID = '0198c000-0000-7000-8000-000000000106'
/** contactID is the id of the contact a linked task names. */
export const contactID = '0198c000-0000-7000-8000-000000000107'
/** linkedContact is the contact a linked task names. */
export const linkedContact = { id: contactID, name: 'Maria Perez' }

/** Returns the local day the given number of days from today as YYYY-MM-DD, apart from the screen's date helpers. */
export function localDate(offsetDays: number) {
	const at = new Date()
	at.setDate(at.getDate() + offsetDays)
	return at.toLocaleDateString('en-CA')
}

/** Returns the due label an overdue row shows for the given YYYY-MM-DD day. */
export function dueLabel(iso: string) {
	return `Due ${iso.split('-').reverse().join('/')}`
}

/** Returns the toast a postpone to the given YYYY-MM-DD day raises. */
export function movedToast(iso: string) {
	return `Task moved to ${iso.split('-').reverse().join('/')}.`
}

/** Returns the toast a quick add for a YYYY-MM-DD day other than the one shown raises. */
export function addedToast(iso: string) {
	return `Task added for ${iso.split('-').reverse().join('/')}.`
}

/** today is the local day the screen opens on. */
export const today = localDate(0)
/** tomorrow is the local day after today. */
export const tomorrow = localDate(1)
/** yesterday is the local day before today. */
export const yesterday = localDate(-1)

/** Returns a task row due today with no time and no contact. */
export function taskRow(id: string, title: string, status = 'open', priority = 0) {
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

/** TaskRow is one task as the mocked server stores it. */
export type TaskRow = ReturnType<typeof taskRow>

/** Returns the task node a mutation answers for the row. */
export function taskNode(row: TaskRow) {
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

/** Returns the task connection listing the rows, with a next page when an end cursor is given. */
export function taskPage(rows: TaskRow[], endCursor: string | null = null) {
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

/** Returns the two open tasks and the done task the day lists before a test says otherwise. */
export function dayTasks() {
	return [
		taskRow(callID, 'Call the supplier'),
		taskRow(quoteID, 'Send the quote'),
		taskRow(doneID, 'Book the courier', 'done'),
	]
}

/** TaskDay reads the rows a test file holds and keeps what the mocked server was asked. */
interface TaskDay {
	tasks: () => TaskRow[]
	overdue: () => TaskRow[]
	listedDates: string[]
	overdueBefore: string[]
	patched: { id: string; body: Record<string, unknown> }[]
	created: Record<string, unknown>[]
}

/** Serves the open and done lists of the day from the rows of each status, keeping the day asked. */
function dayHandler(day: TaskDay) {
	return graphql.query('DayTasks', ({ variables }) => {
		day.listedDates.push(String(variables.date))
		const status = String(variables.status)
		return HttpResponse.json({
			data: { tasks: taskPage(day.tasks().filter((row) => row.status === status)) },
		})
	})
}

/** Serves the overdue list from the rows due before the day asked, keeping that day. */
function overdueHandler(day: TaskDay) {
	return graphql.query('OverdueTasks', ({ variables }) => {
		const dueBefore = String(variables.dueBefore)
		day.overdueBefore.push(dueBefore)
		return HttpResponse.json({ data: { tasks: taskPage(day.overdue().filter((row) => row.due_on < dueBefore)) } })
	})
}

/** Puts the updated row in the place of the row holding its id. */
function replaceRow(rows: TaskRow[], updated: TaskRow) {
	rows.splice(0, rows.length, ...rows.map((row) => (row.id === updated.id ? updated : row)))
}

/** Answers an update from the stored row, keeping the body it was sent. */
function updateHandler(day: TaskDay) {
	return graphql.mutation('UpdateTask', ({ variables }) => {
		const input = variables.input as { status?: string; dueOn?: string; dueTime?: string | null }
		const id = String(variables.id)
		const timed = 'dueTime' in input ? { due_time: input.dueTime } : {}
		day.patched.push({ id, body: { status: input.status, due_on: input.dueOn, ...timed } })
		const stored = [...day.tasks(), ...day.overdue()].find((row) => row.id === id)
		if (stored === undefined) {
			return HttpResponse.json({ data: null, errors: [{ message: 'task: not found' }] })
		}
		const updated = {
			...stored,
			status: input.status ?? stored.status,
			due_on: input.dueOn ?? stored.due_on,
			due_time: input.dueTime === undefined ? stored.due_time : input.dueTime,
		}
		replaceRow(day.tasks(), updated)
		replaceRow(day.overdue(), updated)
		return HttpResponse.json({ data: { updateTask: taskNode(updated) } })
	})
}

/** Answers a create with a new open row on the day, keeping the input it was sent. */
function createHandler(day: TaskDay) {
	return graphql.mutation('CreateTask', ({ variables }) => {
		const input = variables.input as { title: string; dueOn: string; dueTime?: string; priority?: number }
		day.created.push({ ...input })
		const row = {
			...taskRow(addedID, input.title, 'open', input.priority ?? 0),
			due_on: input.dueOn,
			due_time: input.dueTime ?? null,
		}
		day.tasks().push(row)
		return HttpResponse.json({
			data: { createTask: { __typename: 'CreateTaskPayload', task: taskNode(row), replay: false } },
		})
	})
}

/** Serves the open and done lists of each day from the tasks due on it. */
function perDayHandler(tasks: () => TaskRow[]) {
	return graphql.query('DayTasks', ({ variables }) => {
		const date = String(variables.date)
		const status = String(variables.status)
		return HttpResponse.json({
			data: { tasks: taskPage(tasks().filter((row) => row.due_on === date && row.status === status)) },
		})
	})
}

/** Orders two tasks of a day as the server lists them, by priority, by time with an untimed one last, then by id. */
function serverOrder(first: TaskRow, second: TaskRow) {
	const clock = (row: TaskRow) => row.due_time ?? '24:00'
	return (
		second.priority - first.priority ||
		clock(first).localeCompare(clock(second)) ||
		first.id.localeCompare(second.id)
	)
}

/** Serves each day list a page at a time after the cursor it names, in the order the server lists it. */
function rankedHandler(tasks: () => TaskRow[]) {
	return graphql.query('DayTasks', ({ variables }) => {
		const listed = tasks()
			.filter((row) => row.status === String(variables.status))
			.toSorted(serverOrder)
		const start = variables.after ? listed.findIndex((row) => row.id === variables.after) + 1 : 0
		const end = start + Number(variables.first)
		const shown = listed.slice(start, end)
		return HttpResponse.json({
			data: { tasks: taskPage(shown, end < listed.length ? (shown.at(-1)?.id ?? null) : null) },
		})
	})
}

/** Binds the mocked task server to the open and overdue rows a test file holds and to what it was asked. */
export function taskServer(tasks: () => TaskRow[], overdue: () => TaskRow[]) {
	const day: TaskDay = { tasks, overdue, listedDates: [], overdueBefore: [], patched: [], created: [] }
	return {
		listedDates: day.listedDates,
		overdueBefore: day.overdueBefore,
		patched: day.patched,
		created: day.created,
		serveDay: () => {
			for (const asked of [day.listedDates, day.overdueBefore, day.patched, day.created]) {
				asked.length = 0
			}
			server.use(dayHandler(day), overdueHandler(day), updateHandler(day), createHandler(day))
		},
		servePerDay: () => server.use(perDayHandler(tasks)),
		serveRanked: () => server.use(rankedHandler(tasks)),
	}
}

/** Picks a day in the quick add due date field. */
export async function pickDue(day: string) {
	const due = screen.getByLabelText('Due date')
	await userEvent.clear(due)
	await userEvent.type(due, day)
}

/** Types a time in the quick add time field. */
export async function pickTime(time: string) {
	await userEvent.type(screen.getByLabelText('Time'), time)
}

/** Picks the High priority in the quick add priority field. */
export async function pickHigh() {
	await userEvent.click(screen.getByLabelText('Priority'))
	await userEvent.click(await screen.findByRole('option', { name: 'High' }))
}

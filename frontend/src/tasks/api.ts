// SPDX-License-Identifier: AGPL-3.0-or-later

import { UnauthorizedError } from '@gopherium/react-auth'
import { z } from 'zod'

const taskSchema = z.object({
	id: z.string(),
	assignee_id: z.string(),
	contact_id: z.string().nullable(),
	title: z.string(),
	status: z.string(),
	priority: z.number(),
	due_on: z.string(),
	origin_source: z.string().nullable(),
	origin_event_id: z.string().nullable(),
	created_at: z.coerce.date(),
})

export type Task = z.infer<typeof taskSchema>

const taskPageSchema = z.object({
	tasks: z.array(taskSchema),
	next_cursor: z.string().nullable(),
})

export type TaskPage = z.infer<typeof taskPageSchema>

/**
 * ValidationError is thrown when the backend rejects task details.
 */
export class ValidationError extends Error {}

const errorSchema = z.object({ error: z.string() })

/**
 * Reads the backend's error message from a response, with a fallback.
 * @param response - The rejected response.
 * @param fallback - The message used when no error body is readable.
 * @returns The message to show.
 */
async function errorMessage(response: Response, fallback: string): Promise<string> {
	try {
		const parsed = errorSchema.safeParse(await response.json())
		return parsed.success ? parsed.data.error : fallback
	} catch {
		return fallback
	}
}

/**
 * Fetches one page of the tasks due on a day, of every status.
 * @param date - The due date as YYYY-MM-DD.
 * @param cursor - The cursor from the previous page, or an empty string.
 * @returns The parsed page.
 */
export async function fetchDayTasks(date: string, cursor: string): Promise<TaskPage> {
	const params = new URLSearchParams({ date, status: 'all' })
	if (cursor !== '') {
		params.set('cursor', cursor)
	}
	const response = await fetch(`/api/tasks?${params.toString()}`)
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (!response.ok) {
		throw new Error(`listing tasks failed with status ${response.status}`)
	}
	return taskPageSchema.parse(await response.json())
}

/**
 * Fetches one page of the open tasks due before a day.
 * @param date - The exclusive upper bound as YYYY-MM-DD.
 * @param cursor - The cursor from the previous page, or an empty string.
 * @returns The parsed page.
 */
export async function fetchOverdueTasks(date: string, cursor: string): Promise<TaskPage> {
	const params = new URLSearchParams({ due_before: date, status: 'open' })
	if (cursor !== '') {
		params.set('cursor', cursor)
	}
	const response = await fetch(`/api/tasks?${params.toString()}`)
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (!response.ok) {
		throw new Error(`listing overdue tasks failed with status ${response.status}`)
	}
	return taskPageSchema.parse(await response.json())
}

/**
 * Creates a task due on a day and returns it.
 * @param title - The task title.
 * @param dueOn - The due date as YYYY-MM-DD.
 * @returns The created task.
 */
export async function createTask(title: string, dueOn: string): Promise<Task> {
	const response = await fetch('/api/tasks', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ title, due_on: dueOn }),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (response.status === 422) {
		throw new ValidationError(await errorMessage(response, 'invalid task details'))
	}
	if (!response.ok) {
		throw new Error(`creating task failed with status ${response.status}`)
	}
	return taskSchema.parse(await response.json())
}

/**
 * Applies a partial update to a task and returns the updated task.
 * @param id - The task identifier.
 * @param changes - The fields to replace.
 * @returns The updated task.
 */
export async function patchTask(
	id: string,
	changes: { title?: string; due_on?: string; status?: string; priority?: number },
): Promise<Task> {
	const response = await fetch(`/api/tasks/${id}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(changes),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (response.status === 422) {
		throw new ValidationError(await errorMessage(response, 'invalid task details'))
	}
	if (!response.ok) {
		throw new Error(`updating task failed with status ${response.status}`)
	}
	return taskSchema.parse(await response.json())
}

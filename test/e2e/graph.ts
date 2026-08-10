// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect } from '@playwright/test'
import type { APIRequestContext } from '@playwright/test'

/** GraphAnswer is the envelope every graph response carries. */
type GraphAnswer<T> = { data?: T; errors?: { message: string }[] }

/**
 * Posts one graph operation and returns its data, failing the test on any error.
 * @param request - The Playwright request context carrying the credential.
 * @param query - The operation document.
 * @param variables - The operation variables.
 * @returns The data the operation answered with.
 */
export async function graph<T>(
	request: APIRequestContext,
	query: string,
	variables: Record<string, unknown> = {},
): Promise<T> {
	const response = await request.post('/api/graphql', { data: { query, variables } })
	expect(response.status(), await response.text()).toBe(200)
	const answer = (await response.json()) as GraphAnswer<T>
	expect(answer.errors, JSON.stringify(answer.errors)).toBeUndefined()
	return answer.data as T
}

/**
 * Uploads a file through the graph, following the multipart request specification.
 * @param request - The Playwright request context carrying the credential.
 * @param query - The mutation document taking one Upload variable named file.
 * @param file - The file to send.
 * @returns The data the mutation answered with.
 */
export async function graphUpload<T>(
	request: APIRequestContext,
	query: string,
	file: { name: string; mimeType: string; buffer: Buffer },
): Promise<T> {
	const response = await request.post('/api/graphql', {
		multipart: {
			operations: JSON.stringify({ query, variables: { file: null } }),
			map: JSON.stringify({ upload: ['variables.file'] }),
			upload: file,
		},
	})
	expect(response.status(), await response.text()).toBe(200)
	const answer = (await response.json()) as GraphAnswer<T>
	expect(answer.errors, JSON.stringify(answer.errors)).toBeUndefined()
	return answer.data as T
}

/**
 * Creates a task through the graph.
 * @param request - The Playwright request context carrying the credential.
 * @param input - The CreateTaskInput fields.
 * @returns The stored task id beside whether the create was a replay.
 */
export async function createTask(
	request: APIRequestContext,
	input: Record<string, unknown>,
): Promise<{ id: string; replay: boolean }> {
	const data = await graph<{ createTask: { replay: boolean; task: { id: string } } }>(
		request,
		'mutation($input: CreateTaskInput!) { createTask(input: $input) { replay task { id } } }',
		{ input },
	)
	return { id: data.createTask.task.id, replay: data.createTask.replay }
}

/**
 * Reschedules a task through the graph.
 * @param request - The Playwright request context carrying the credential.
 * @param id - The task to move.
 * @param dueOn - The day to move it to.
 */
export async function rescheduleTask(
	request: APIRequestContext,
	id: string,
	dueOn: string,
): Promise<void> {
	await graph(
		request,
		'mutation($id: UUID!, $input: UpdateTaskInput!) { updateTask(id: $id, input: $input) { id } }',
		{ id, input: { dueOn } },
	)
}

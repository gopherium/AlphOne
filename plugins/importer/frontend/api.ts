// SPDX-License-Identifier: AGPL-3.0-or-later

import { ValidationError } from '@alphone/frontend-sdk'
import { UnauthorizedError } from '@gopherium/react-auth'
import { z } from 'zod'

const base = '/api/plugins/importer'

const fieldSchema = z.object({
	name: z.string(),
	label: z.string(),
	required: z.boolean(),
})

export type ImportField = z.infer<typeof fieldSchema>

const summarySchema = z.object({
	id: z.string(),
	user_id: z.string(),
	filename: z.string(),
	state: z.string(),
	row_count: z.number(),
	imported_count: z.number(),
	skipped_count: z.number(),
	failed_count: z.number(),
	created_at: z.coerce.date(),
})

const detailSchema = summarySchema.extend({
	columns: z.array(z.string()),
	mapping: z.record(z.string(), z.string()),
})

export type ImportDetail = z.infer<typeof detailSchema>

const rowSchema = z.object({
	id: z.string(),
	position: z.number(),
	cells: z.array(z.string()),
	outcome: z.string(),
	reason: z.string().nullable(),
	contact_id: z.string().nullable(),
})

export type ImportRow = z.infer<typeof rowSchema>

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
 * Refuses a response the caller cannot use.
 * @param response - The response to inspect.
 * @param fallback - The message shown when the body carries none.
 */
async function refuse(response: Response, fallback: string): Promise<void> {
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (response.status === 422 || response.status === 409) {
		throw new ValidationError(await errorMessage(response, fallback))
	}
	if (!response.ok) {
		throw new Error(`${fallback} (status ${response.status})`)
	}
}

/**
 * Fetches the fields a column may be mapped onto.
 * @returns The mappable fields.
 */
export async function fetchFields(): Promise<ImportField[]> {
	const response = await fetch(`${base}/fields`)
	await refuse(response, 'the fields could not be read')
	return z.array(fieldSchema).parse(await response.json())
}

/**
 * Fetches one import with its columns and mapping.
 * @param id - The import identifier.
 * @returns The parsed import.
 */
export async function fetchImport(id: string): Promise<ImportDetail> {
	const response = await fetch(`${base}/imports/${id}`)
	await refuse(response, 'the import could not be read')
	return detailSchema.parse(await response.json())
}

/**
 * Fetches the staged rows of one import.
 * @param id - The import identifier.
 * @returns The staged rows.
 */
export async function fetchRows(id: string): Promise<ImportRow[]> {
	const response = await fetch(`${base}/imports/${id}/rows`)
	await refuse(response, 'the rows could not be read')
	return z.array(rowSchema).parse(await response.json())
}

/**
 * Uploads a file as a staged import.
 * @param file - The chosen CSV or Excel file.
 * @returns The stored import.
 */
export async function uploadImport(file: File): Promise<ImportDetail> {
	const body = new FormData()
	body.append('file', file)
	const response = await fetch(`${base}/imports`, { method: 'POST', body })
	await refuse(response, 'the file could not be imported')
	return detailSchema.parse(await response.json())
}

/**
 * Stores the column assignments of an import.
 * @param id - The import identifier.
 * @param assignments - The column to field pairs.
 */
export async function saveMapping(
	id: string,
	assignments: { column: number; field: string }[],
): Promise<void> {
	const response = await fetch(`${base}/imports/${id}/mapping`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ assignments }),
	})
	await refuse(response, 'the mapping could not be saved')
}

/**
 * Turns the staged rows of an import into contacts.
 * @param id - The import identifier.
 */
export async function commitImport(id: string): Promise<void> {
	const response = await fetch(`${base}/imports/${id}/commit`, { method: 'POST' })
	await refuse(response, 'the import could not be committed')
}

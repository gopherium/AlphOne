// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { ValidationError } from '@alphone/frontend-sdk'
import { UnauthorizedError } from '@gopherium/react-auth'
import { expect, test } from 'vitest'

import {
	commitImport,
	saveMapping,
	uploadImport,
} from '../api'
import { importID } from '../handlers'

const base = '/api/plugins/importer'

test('uploadImport posts the file as multipart form data', async () => {
	let sent = false
	server.use(
		http.post(`${base}/imports`, async ({ request }) => {
			sent =
				(request.headers.get('content-type') ?? '').startsWith('multipart/form-data') &&
				(await request.text()).includes('name="file"')
			return HttpResponse.json(
				{
					id: importID,
					user_id: '019f5a00-0000-7000-8000-0000000000aa',
					filename: 'contacts.csv',
					state: 'ready',
					row_count: 1,
					imported_count: 0,
					skipped_count: 0,
					failed_count: 0,
					created_at: '2026-08-01T10:00:00Z',
					columns: ['Name'],
					mapping: {},
				},
				{ status: 201 },
			)
		}),
	)

	const stored = await uploadImport(new File(['Name\nMaria Perez\n'], 'contacts.csv'))

	expect(sent).toBe(true)
	expect(stored.filename).toBe('contacts.csv')
})

test('saveMapping sends the assignments', async () => {
	let body: unknown = null
	server.use(
		http.put(`${base}/imports/:id/mapping`, async ({ request }) => {
			body = await request.json()
			return new HttpResponse(null, { status: 204 })
		}),
	)

	await saveMapping(importID, [{ column: 0, field: 'name' }])

	expect(body).toEqual({ assignments: [{ column: 0, field: 'name' }] })
})

test('commitImport asks for the commit', async () => {
	let committed = false
	server.use(
		http.post(`${base}/imports/:id/commit`, () => {
			committed = true
			return HttpResponse.json({ id: importID, imported: 1, skipped: 0, failed: 0 })
		}),
	)

	await commitImport(importID)

	expect(committed).toBe(true)
})

test('an unauthorized read drops the session', async () => {
	server.use(
		http.post(`${base}/imports/:id/commit`, () =>
			HttpResponse.json({ error: 'no session' }, { status: 401 }),
		),
	)

	await expect(commitImport(importID)).rejects.toBeInstanceOf(UnauthorizedError)
})

test('a refused mapping carries the backend message', async () => {
	server.use(
		http.put(`${base}/imports/:id/mapping`, () =>
			HttpResponse.json({ error: 'the registry carries no field "nickname"' }, { status: 422 }),
		),
	)

	await expect(saveMapping(importID, [])).rejects.toThrow(
		'the registry carries no field "nickname"',
	)
})

test('a conflicting commit carries the backend message', async () => {
	server.use(
		http.post(`${base}/imports/:id/commit`, () =>
			HttpResponse.json({ error: 'the import is already committed' }, { status: 409 }),
		),
	)

	await expect(commitImport(importID)).rejects.toBeInstanceOf(ValidationError)
})

test('a refusal without a readable body falls back', async () => {
	server.use(
		http.post(`${base}/imports/:id/commit`, () => new HttpResponse('not json', { status: 422 })),
	)

	await expect(commitImport(importID)).rejects.toThrow('the import could not be committed')
})

test('a refusal whose body carries no message falls back', async () => {
	server.use(
		http.post(`${base}/imports/:id/commit`, () => HttpResponse.json({ oops: true }, { status: 422 })),
	)

	await expect(commitImport(importID)).rejects.toThrow('the import could not be committed')
})

test('any other failure reports its status', async () => {
	server.use(
		http.post(`${base}/imports/:id/commit`, () =>
			HttpResponse.json({ error: 'boom' }, { status: 500 }),
		),
	)

	await expect(commitImport(importID)).rejects.toThrow(
		'the import could not be committed (status 500)',
	)
})

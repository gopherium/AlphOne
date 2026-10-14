// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import { expect, test, vi } from 'vitest'

import { importID } from '../handlers'
import { plugin } from '../index'
import { importCommitMutation, importDetailQuery, importRowsQuery } from '../operations'

test('grants the area its imports are held to, named in English until a catalogue loads', () => {
	expect(plugin.tokenAreas).toEqual([{ value: 'imports', label: 'imports' }])
})

test('names its area in Spanish in its own catalogue', async () => {
	const catalog = await plugin.locale?.load('es-ES')

	expect((catalog?.['token area\u0004imports'] as string[] | undefined)?.[0]).toBe('importaciones')
})

test('keys the import types that carry no id, a field by its name and a created contact by its row', () => {
	expect(Object.keys(plugin.graphKeys ?? {}).sort()).toEqual([
		'ImportAssignment',
		'ImportCommitPayload',
		'ImportContact',
		'ImportField',
		'ImportRowReason',
	])
	expect(plugin.graphKeys?.ImportAssignment({})).toBeNull()
	expect(plugin.graphKeys?.ImportCommitPayload({})).toBeNull()
	expect(plugin.graphKeys?.ImportRowReason({})).toBeNull()
	expect(plugin.graphKeys?.ImportField({ name: 'email' })).toBe('email')
	expect(plugin.graphKeys?.ImportContact({ rowId: 'id-row' })).toBe('id-row')
})

test('a client keying the plugin caches an import, its rows and its commit without warning', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	server.use(
		graphql.query('ImportDetail', () =>
			HttpResponse.json({
				data: {
					importJob: {
						__typename: 'ImportJob',
						id: importID,
						filename: 'contacts.csv',
						state: 'ready',
						columns: ['Name'],
						mapping: [{ __typename: 'ImportAssignment', column: 0, field: 'name' }],
					},
					importFields: [{ __typename: 'ImportField', name: 'name', label: 'Name', required: true }],
				},
			}),
		),
		graphql.query('ImportRows', () =>
			HttpResponse.json({
				data: {
					importJob: {
						__typename: 'ImportJob',
						id: importID,
						rows: [
							{
								__typename: 'ImportRow',
								id: 'id-row',
								position: 1,
								cells: ['Maria Perez'],
								outcome: 'skipped',
								reason: {
									__typename: 'ImportRowReason',
									code: 'identity_taken_by',
									meta: { ownerName: 'Maria Perez' },
								},
							},
						],
					},
				},
			}),
		),
		graphql.mutation('ImportCommit', () =>
			HttpResponse.json({
				data: {
					importCommit: { __typename: 'ImportCommitPayload', id: importID, imported: 0, skipped: 1, failed: 0 },
				},
			}),
		),
	)
	const { graph } = fakeGraphClient([plugin])

	const detail = await graph.client.query(importDetailQuery, { id: importID }).toPromise()
	const rows = await graph.client.query(importRowsQuery, { id: importID, limit: 10, offset: 0 }).toPromise()
	const committed = await graph.client.mutation(importCommitMutation, { id: importID }).toPromise()

	expect(detail.error).toBeUndefined()
	expect(rows.data?.importJob?.rows[0]?.reason?.code).toBe('identity_taken_by')
	expect(committed.error).toBeUndefined()
	expect(warn).not.toHaveBeenCalled()
})

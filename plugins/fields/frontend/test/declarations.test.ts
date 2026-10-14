// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import { expect, test, vi } from 'vitest'

import { plugin } from '../index'
import { fieldsQuery } from '../operations'

test('grants the area its fields are held to, named in English until a catalogue loads', () => {
	expect(plugin.tokenAreas).toEqual([{ value: 'fields', label: 'fields' }])
})

test('names its area in Spanish in its own catalogue', async () => {
	const catalog = await plugin.locale?.load('es-ES')

	expect((catalog?.['token area\u0004fields'] as string[] | undefined)?.[0]).toBe('campos')
})

test('keys the sub-fields of a definition, which carry no id', () => {
	expect(Object.keys(plugin.graphKeys ?? {})).toEqual(['FieldSubField'])
	expect(plugin.graphKeys?.FieldSubField({})).toBeNull()
})

test('a client keying the plugin caches the field catalogue without warning', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	server.use(
		graphql.query('Fields', () =>
			HttpResponse.json({
				data: {
					fields: [
						{
							__typename: 'FieldDefinition',
							id: 'id-history',
							name: 'history',
							label: 'History',
							kind: 'REPEATER',
							subFields: [{ __typename: 'FieldSubField', name: 'date', label: 'Date', kind: 'DATE' }],
						},
					],
				},
			}),
		),
	)
	const { graph } = fakeGraphClient([plugin])

	const result = await graph.client.query(fieldsQuery, {}).toPromise()

	expect(result.error).toBeUndefined()
	expect(result.data?.fields[0]?.subFields?.[0]?.name).toBe('date')
	expect(warn).not.toHaveBeenCalled()
})

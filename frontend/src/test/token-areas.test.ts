// SPDX-License-Identifier: AGPL-3.0-or-later

import type { TokenArea } from '@alphone/frontend-sdk'
import { expect, test } from 'vitest'

import { grantableAreas } from '../users/tokenFormat'

const schema = Object.values(
	import.meta.glob('../../../graph/schema.graphql', {
		query: '?raw',
		import: 'default',
		eager: true,
	}),
)[0] as string

const publicSchemas = Object.values(
	import.meta.glob(['../../../graph/schema/*.graphqls', '../../../plugins/*/graph/*.graphqls'], {
		query: '?raw',
		import: 'default',
		eager: true,
	}),
) as string[]

/**
 * areasIn names every area the given schema texts hold a root field in.
 */
function areasIn(texts: readonly string[]): Set<string> {
	const found = new Set<string>()
	for (const text of texts) {
		for (const match of text.matchAll(/@scope\(area: "([^"]+)"/g)) {
			found.add(match[1])
		}
	}
	return found
}

/**
 * declaredAreas names every area the generated schema holds a root field in.
 */
function declaredAreas(): Set<string> {
	return areasIn([schema])
}

/**
 * Returns the values of the areas a list holds, in order.
 * @param areas - The areas.
 * @returns The values.
 */
function valuesOf(areas: readonly TokenArea[]): string[] {
	return areas.map((area) => area.value)
}

test('the mint form offers only areas the schema declares', () => {
	const declared = declaredAreas()

	expect(declared.size).toBeGreaterThan(0)
	for (const area of grantableAreas()) {
		expect(declared).toContain(area.value)
	}
})

test('the mint form offers no area the gate answers on its own', () => {
	for (const area of grantableAreas()) {
		expect(area.value).not.toBe('auth')
		expect(area.value).not.toBe('tokens')
	}
})

test('the mint form offers every area the core and the plugins declare, auth and tokens aside', () => {
	const declared = [...areasIn(publicSchemas)].filter((area) => area !== 'auth' && area !== 'tokens')

	expect(declared.length).toBeGreaterThan(0)
	expect(valuesOf(grantableAreas())).toEqual(expect.arrayContaining(declared))
})

test('a plugin area joins the core ones in order of their names', () => {
	const areas = grantableAreas([{ id: 'probe', tokenAreas: [{ value: 'notes', label: 'notes' }] }, { id: 'bare' }])

	expect(valuesOf(areas)).toEqual(['contacts', 'events', 'meta', 'notes', 'tasks', 'users', 'webhooks'])
})

test('a plugin area keeps the label it reads fresh', () => {
	const notes = {
		value: 'notes',
		get label() {
			return 'notes, read now'
		},
	}

	const areas = grantableAreas([{ id: 'probe', tokenAreas: [notes] }])

	expect(areas.find((area) => area.value === 'notes')).toBe(notes)
})

test('refuses an area two plugins grant, naming both', () => {
	expect(() =>
		grantableAreas([
			{ id: 'alpha', tokenAreas: [{ value: 'notes', label: 'notes' }] },
			{ id: 'beta', tokenAreas: [{ value: 'notes', label: 'notes' }] },
		]),
	).toThrow('tokens: beta grants the area notes, which alpha already grants')
})

test('refuses a plugin granting an area the core grants', () => {
	expect(() => grantableAreas([{ id: 'alpha', tokenAreas: [{ value: 'contacts', label: 'contacts' }] }])).toThrow(
		'tokens: alpha grants the area contacts, which the core already grants',
	)
})

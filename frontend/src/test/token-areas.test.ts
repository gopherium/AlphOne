// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { grantableAreas } from '../users/tokenFormat'

const schema = Object.values(
	import.meta.glob('../../../graph/schema.graphql', {
		query: '?raw',
		import: 'default',
		eager: true,
	}),
)[0] as string

/**
 * declaredAreas names every area the generated schema holds a root field in.
 */
function declaredAreas(): Set<string> {
	const found = new Set<string>()
	for (const match of schema.matchAll(/@scope\(area: "([^"]+)"/g)) {
		found.add(match[1])
	}
	return found
}

test('the mint form offers only areas the schema declares', () => {
	const declared = declaredAreas()

	expect(declared.size).toBeGreaterThan(0)
	for (const area of grantableAreas) {
		expect(declared).toContain(area)
	}
})

test('the mint form offers no area the gate answers on its own', () => {
	for (const area of grantableAreas) {
		expect(area).not.toBe('auth')
		expect(area).not.toBe('tokens')
	}
})

test('the mint form offers the area the whatsapp routes are held to', () => {
	expect(grantableAreas).toContain('whatsapp')
})

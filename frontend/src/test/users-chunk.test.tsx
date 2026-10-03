// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { createAppRouter } from '../router'

test('loads the users list in a chunk of its own, off the first paint', () => {
	const { component } = createAppRouter().routesById['/users'].options

	expect(typeof (component as { preload?: unknown }).preload).toBe('function')
})

test('loads the tokens list in a chunk of its own, off the first paint', () => {
	const { component } = createAppRouter().routesById['/users/tokens'].options

	expect(typeof (component as { preload?: unknown }).preload).toBe('function')
})

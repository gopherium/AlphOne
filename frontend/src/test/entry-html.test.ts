// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

const sources = import.meta.glob('../../index.html', {
	query: '?raw',
	import: 'default',
	eager: true,
}) as Record<string, string>

const entry = sources['../../index.html']

test('draws nothing in the root before any script runs', () => {
	expect(entry).toMatch(/<div id="root"><\/div>/)
})

test('carries no style of its own', () => {
	expect(entry).not.toMatch(/<style/)
})

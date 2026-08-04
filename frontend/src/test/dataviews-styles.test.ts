// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

const sources = import.meta.glob('../../../sdk/frontend/dataviews.ts', {
	query: '?raw',
	import: 'default',
	eager: true,
}) as Record<string, string>

test('the dataviews subpath carries the stylesheets it cannot render without', () => {
	const source = Object.values(sources)[0]

	expect(source).toBeDefined()
	expect(source).toContain("import '@wordpress/components/build-style/style.css'")
	expect(source).toContain("import '@wordpress/dataviews/build-style/style.css'")
})

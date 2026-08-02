// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

const sources = import.meta.glob(
	['../**/*.tsx', '../../../plugins/*/frontend/**/*.tsx', '../../../sdk/frontend/**/*.tsx'],
	{ query: '?raw', import: 'default', eager: true },
) as Record<string, string>

const exempt = /\.test\.tsx$/

/**
 * Returns every screen file whose source matches the given pattern.
 * @param pattern - The pattern a conformant screen never contains.
 * @returns The offending paths.
 */
function filesMatching(pattern: RegExp): string[] {
	return Object.entries(sources)
		.filter(([path]) => !exempt.test(path))
		.filter(([, source]) => pattern.test(source))
		.map(([path]) => path.replace(/^(\.\.\/)+/, ''))
		.sort()
}

// A conformant screen writes render={<h1 />}, which is self closing and so
// never produces a closing tag.
test('screens render their title through the page template, never a raw h1', () => {
	expect(filesMatching(/<\/h1>/)).toEqual([])
})

test('screens render section headings through the design system, never a raw h2', () => {
	expect(filesMatching(/<\/h2>/)).toEqual([])
})

test('the page title is the only first level heading, so the shell owns none', () => {
	const owner = 'sdk/frontend/PageScreen.tsx'
	expect(filesMatching(/render=\{<h1 \/>\}/).filter((path) => path !== owner)).toEqual([])
})

// WPDS caps the empty state at 320px and leaves placing it to the consumer, so
// every screen centers it through the same class.
test('screens center their empty state through the template class', () => {
	expect(filesMatching(/<EmptyState\.Root>/)).toEqual([])
})

// A table is wider than a phone, so it scrolls inside its own region instead of
// dragging the page sideways.
test('screens wrap their table in the scrolling region', () => {
	const withTable = filesMatching(/className="alphone-table"/)
	const withRegion = filesMatching(/className="alphone-table-scroll"/)
	expect(withTable.filter((path) => !withRegion.includes(path))).toEqual([])
})

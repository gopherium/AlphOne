// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

const sources = import.meta.glob(
	['../**/*.tsx', '../../../plugins/*/frontend/**/*.tsx', '../../../sdk/frontend/**/*.tsx'],
	{ query: '?raw', import: 'default', eager: true },
) as Record<string, string>

// The shell owns the brand heading, which U5 demotes out of the outline.
const exempt = /Layout\.tsx$|\.test\.tsx$/

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

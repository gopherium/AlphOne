// SPDX-License-Identifier: AGPL-3.0-or-later

import { calendar, comment, commentAuthorAvatar, language, list, people, upload } from '@alphone/frontend-sdk'
import { render, screen, within } from '@testing-library/react'
import type { ReactElement } from 'react'
import { expect, test } from 'vitest'

import { everyMenuItem, fullAccessSession } from './navSession'
import { renderAt } from './render'

const ROW = 'godmin-navigation-item'

/**
 * Returns what a glyph draws, read from the row holding it or from the glyph itself.
 * @param held - The row link holding the glyph, or the glyph.
 * @returns The markup inside its svg.
 */
function drawingOf(held: Element | ReactElement): string | undefined {
	const element = held instanceof Element ? held : render(held).container
	return element.querySelector('svg')?.innerHTML
}

test('dresses a core row and a plugin row like every other sidebar row', async () => {
	renderAt('/tasks')

	const sidebar = await screen.findByRole('region', { name: 'Navigation' })
	expect(within(sidebar).getByRole('link', { name: 'Tasks' })).toHaveClass(ROW)
	expect(within(sidebar).getByRole('link', { name: 'WhatsApp' })).toHaveClass(ROW)
})

test('lists every sidebar row as an item of one list, the core rows first', async () => {
	renderAt('/tasks', fullAccessSession)

	const sidebar = await screen.findByRole('region', { name: 'Navigation' })
	const rows = within(within(sidebar).getByRole('list')).getAllByRole('listitem')
	expect(rows.map((row) => row.textContent)).toEqual(everyMenuItem.map((item) => item.label))
	expect(rows.slice(0, 4).map((row) => row.textContent)).toEqual(['Tasks', 'Contacts', 'Users', 'Language'])
})

test('draws each sidebar row with the glyph WordPress draws for it', async () => {
	renderAt('/tasks')

	const sidebar = within(await screen.findByRole('region', { name: 'Navigation' }))
	const glyphs: [string, ReactElement][] = [
		['Tasks', calendar],
		['Contacts', commentAuthorAvatar],
		['Users', people],
		['Language', language],
		['Fields', list],
		['Import', upload],
		['WhatsApp', comment],
	]
	for (const [name, glyph] of glyphs) {
		expect(drawingOf(sidebar.getByRole('link', { name })), name).toBe(drawingOf(glyph))
	}
})

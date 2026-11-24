// SPDX-License-Identifier: AGPL-3.0-or-later

import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { declarationsIn } from '@alphone/frontend-sdk/testing'
import { expect, test } from 'vitest'

/**
 * Returns the text of one file of the plugin's frontend.
 * @param name - The file name, read from the plugin's frontend folder.
 * @returns The text.
 */
function source(name: string): string {
	return readFileSync(join(import.meta.dirname, '..', name), 'utf8')
}

/**
 * Returns the declarations of the rule for exactly one selector in the email sheet.
 * @param selector - The selector of the rule.
 * @param media - The condition of the media block holding the rule, the top level when absent.
 * @returns The declarations, keyed by property.
 */
function declarations(selector: string, media?: string): Record<string, string> {
	return declarationsIn(source('email.css'), selector, media)
}

/** narrow is the condition of the phone block, the width the inbox keeps the sender column alone at. */
const narrow = '(max-width: 782px)'

/** tablet is the condition of the block from the phone width to the wide breakpoint, where the sender cap shrinks. */
const tablet = '(min-width: 783px) and (max-width: 1279px)'

/**
 * Returns every selector of a sheet's rules, comments and media conditions left out.
 * @param sheet - The text of the stylesheet.
 * @returns The selectors, each part of a selector list on its own, in the order the sheet names them.
 */
function selectorsIn(sheet: string): string[] {
	const heads = [...sheet.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{};]+)\{/g)].map((match) => match[1].trim())
	return heads.filter((head) => !head.startsWith('@')).flatMap((head) => head.split(',').map((part) => part.trim()))
}

test('draws the row above the inbox as the list header row: 48px at least, 20px at the sides, a rule under it', () => {
	const stroke = '--wpds-color-stroke-surface-neutral'

	expect(declarations('.alphone-email-inbox__header')).toEqual({
		'box-sizing': 'border-box',
		'flex-shrink': '0',
		'min-block-size': '48px',
		'padding-inline': '20px',
		'border-block-end': `1px solid var(${stroke})`,
	})
})

test('stacks the subject line over the preview, the cell filling its column and giving way to it', () => {
	const gap = '--wpds-dimension-gap-xs'

	expect(declarations('.alphone-email-inbox__cell')).toEqual({
		display: 'flex',
		'flex-direction': 'column',
		gap: `var(${gap})`,
		'min-inline-size': '0',
		'max-inline-size': '100%',
	})
})

test('caps the address under the sender at 22em like the sender line, so the sender column stops too', () => {
	expect(declarations('.alphone-email-inbox__address')).toEqual({ 'max-inline-size': '22em' })
})

test('lets the box around each column cell give way to its column, so the subject cuts at the column edge', () => {
	expect(declarations('.alphone-email-inbox td > .dataviews-view-table__cell-content-wrapper')).toEqual({
		'min-inline-size': '0',
		'max-inline-size': 'none',
	})
})

test('draws the subject line of the cell in the normal foreground past the sender cap, its marks free to wrap', () => {
	const normal = '--wpds-color-foreground-content-neutral'

	expect(declarations('.alphone-email-inbox__cell > .alphone-email-inbox__line')).toEqual({
		'flex-wrap': 'wrap',
		'max-inline-size': 'none',
		color: `var(${normal})`,
	})
})

test('keeps a subject 4em wide beside its marks and no wider than its words, so the marks drop under it first', () => {
	const subject = '.alphone-email-inbox__cell > .alphone-email-inbox__line > .alphone-email-inbox__text'

	expect(declarations(subject)).toEqual({ flex: '1 1 4em', 'max-inline-size': 'max-content' })
})

test('sets a line and the marks beside it in a row capped at the same width', () => {
	const gap = '--wpds-dimension-gap-xs'

	expect(declarations('.alphone-email-inbox__line')).toEqual({
		display: 'flex',
		'align-items': 'center',
		gap: `var(${gap})`,
		'max-inline-size': '22em',
	})
})

test('keeps the marks beside a line whole while the line gives way', () => {
	expect(declarations('.alphone-email-inbox__line > :not(.alphone-email-inbox__text)')).toEqual({ flex: 'none' })
})

test('sets the badges and the file glyph of a line in one box centred on it, so they wrap together', () => {
	const gap = '--wpds-dimension-gap-xs'

	expect(declarations('.alphone-email-inbox__marks')).toEqual({
		display: 'inline-flex',
		'align-items': 'center',
		gap: `var(${gap})`,
	})
})

test('sets the file glyph in a box of its own size, so it centres on its line and adds nothing to the row', () => {
	expect(declarations('.alphone-email-inbox__file')).toEqual({ display: 'inline-flex' })
})

test('cuts a line too long for its cell with an ellipsis', () => {
	expect(declarations('.alphone-email-inbox__text')).toEqual({
		overflow: 'hidden',
		'text-overflow': 'ellipsis',
		'white-space': 'nowrap',
	})
})

test('draws the address and the preview in the weak foreground', () => {
	const weak = '--wpds-color-foreground-content-neutral-weak'

	expect(declarations('.alphone-email-inbox__weak')).toEqual({ color: `var(${weak})` })
})

test('gives the no mailbox state a callout 320px tall at least, padded 64px and 32px, with no side margin', () => {
	expect(declarations('.alphone-email-inbox__callout')).toEqual({
		'min-block-size': '320px',
		'padding-block': '64px',
		'padding-inline': '32px',
		'margin-inline': '0',
	})
})

test('starts every rule with a class of its own, so no rule reaches a plugin that loaded its sheet first', () => {
	const selectors = selectorsIn(source('email.css'))

	expect(selectors.length).toBeGreaterThan(0)
	expect(selectors.filter((selector) => !selector.startsWith('.alphone-email'))).toEqual([])
})

test('keeps the width blocks at the end of the sheet, the phone block last at the width the inbox turns narrow', () => {
	const sheet = source('email.css')
	const blocks = [...sheet.matchAll(/@media[^{]*\{/g)]
	const start = blocks[0]?.index ?? sheet.length

	expect(blocks.map((block) => block[0])).toEqual([`@media ${tablet} {`, `@media ${narrow} {`])
	expect(/\n[^\t\n}@]/.test(sheet.slice(start + 1))).toBe(false)
	expect(source('EmailInboxScreen.tsx')).toContain(`const NARROW = '${narrow}'`)
})

test('caps the sender and its address at 14em from 783px to 1279px, so the subject keeps room for its marks', () => {
	expect(declarations('.alphone-email-inbox__line', tablet)).toEqual({ 'max-inline-size': '14em' })
	expect(declarations('.alphone-email-inbox__address', tablet)).toEqual({ 'max-inline-size': '14em' })
})

test('lets each line of the narrow sender cell fill it in place of a cap, so a phone never scrolls sideways', () => {
	expect(declarations('.alphone-email-inbox .dataviews-view-table__primary-column-content', narrow)).toEqual({
		'flex-grow': '1',
	})
	expect(declarations('.alphone-email-inbox__line', narrow)).toEqual({
		'inline-size': '0',
		'min-inline-size': '100%',
	})
})

test("lets the narrow subject line's marks overhang it by 2px, so a badge row grows as much as a Sent row", () => {
	expect(declarations('.alphone-email-inbox__marks', narrow)).toEqual({ 'margin-block': '-2px' })
})

test('loads with the inbox screen alone, never with the entry the first paint holds', () => {
	expect(source('EmailInboxScreen.tsx')).toContain("import './email.css'")
	for (const eager of ['index.ts', 'nav.ts', 'routes.tsx']) {
		expect({ eager, imports: source(eager).includes('email.css') }).toEqual({ eager, imports: false })
	}
})

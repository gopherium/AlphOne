// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { declarations } from '../../test/stylesheet'

test('sets a menu row as the Site Editor sets a navigation item, a 13px label on a 40px row', () => {
	expect(declarations('.alphone-menu__item')).toMatchObject({
		'box-sizing': 'border-box',
		'min-height': '40px',
		padding: '8px 6px 8px 16px',
		'font-size': expect.stringMatching(/^var\(--wpds-typography-font-size-md\)$/),
		color: expect.stringMatching(/^var\(--wpds-color-foreground-interactive-neutral-weak\)$/),
	})
})

test('sets the current row in the emphasis weight on the interactive fill', () => {
	expect(declarations(".alphone-menu__item[data-status='active']")).toMatchObject({
		'font-weight': expect.stringMatching(/^var\(--wpds-typography-font-weight-emphasis\)$/),
		background: expect.stringMatching(/^var\(--wpds-color-background-interactive-neutral-weak-active\)$/),
		color: expect.stringMatching(/^var\(--wpds-color-foreground-interactive-neutral-weak-active\)$/),
	})
})

test('changes only the text colour of a row under the pointer', () => {
	expect(declarations('.alphone-menu__item:hover')).toEqual({
		color: expect.stringMatching(/^var\(--wpds-color-foreground-interactive-neutral-weak-active\)$/),
	})
})

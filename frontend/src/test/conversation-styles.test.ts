// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { declarations } from './stylesheet'

test('stacks the two lines of a conversation row and leaves the row itself to the sidebar', () => {
	expect(declarations('.alphone-conversation')).toEqual({
		display: 'flex',
		'flex-direction': 'column',
		gap: '2px',
	})
})

test('draws the contact name in the content colour, so it reads before the preview as a list title does', () => {
	expect(declarations('.alphone-conversation__name')).toMatchObject({
		color: expect.stringMatching(/^var\(--wpds-color-foreground-content-neutral\)$/),
	})
})

test('keeps the preview at the default weight, so only the top line of the open conversation turns bold', () => {
	expect(declarations('.alphone-conversation__preview')).toMatchObject({
		'font-weight': expect.stringMatching(/^var\(--wpds-typography-font-weight-default\)$/),
	})
})

test('draws no hover, focus or current state of its own on a conversation row', () => {
	expect(declarations('.alphone-conversation:hover')).toEqual({})
	expect(declarations('.alphone-conversation:focus-visible')).toEqual({})
	expect(declarations(".alphone-conversation[data-status='active']")).toEqual({})
	expect(declarations('.alphone-conversations')).toEqual({})
})

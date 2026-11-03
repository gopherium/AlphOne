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

test('cuts a long number on its own line, so it never widens a row on a phone', () => {
	expect(declarations('.alphone-conversation__number')).toMatchObject({
		overflow: 'hidden',
		'text-overflow': 'ellipsis',
		'white-space': 'nowrap',
		'font-weight': expect.stringMatching(/^var\(--wpds-typography-font-weight-default\)$/),
	})
})

test('cuts a long number in the thread header, so it never widens the canvas on a phone', () => {
	expect(declarations('.alphone-thread__number')).toMatchObject({
		display: 'block',
		overflow: 'hidden',
		'text-overflow': 'ellipsis',
		'white-space': 'nowrap',
	})
})

test('holds the number filter apart from the rows under it', () => {
	expect(declarations('.alphone-conversation-filter')).toEqual({
		'margin-block-end': expect.stringMatching(/^var\(--wpds-dimension-gap-sm\)$/),
	})
})

test('draws no hover, focus or current state of its own on a conversation row', () => {
	expect(declarations('.alphone-conversation:hover')).toEqual({})
	expect(declarations('.alphone-conversation:focus-visible')).toEqual({})
	expect(declarations(".alphone-conversation[data-status='active']")).toEqual({})
	expect(declarations('.alphone-conversations')).toEqual({})
})

// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { declarations } from './stylesheet'

test('fades a done task and strikes through its title alone, never the contact name beside it', () => {
	const weak = '--wpds-color-foreground-content-neutral-weak'

	expect(declarations('.alphone-tasks__title--done')).toEqual({
		color: `var(${weak})`,
	})
	expect(declarations('.alphone-tasks__title--done .alphone-tasks__title-link')).toEqual({
		'text-decoration': 'line-through',
	})
})

test('sets the contact link on the title line, its focus ring clear of the last word, without growing the line', () => {
	const gap = '--wpds-dimension-gap-sm'
	const line = '--wpds-typography-line-height-sm'
	const button = '--wpds-dimension-size-sm'

	expect(declarations('.alphone-tasks__open-contact')).toMatchObject({
		'vertical-align': 'top',
		'max-width': `calc(100% - var(${gap}))`,
		'margin-inline-start': `var(${gap})`,
		'margin-block': `calc((var(${line}) - var(${button})) / 2)`,
	})
})

test('draws the contact name as a small quiet note beside the title, regular weight in the weak colour', () => {
	const weak = '--wpds-color-foreground-content-neutral-weak'
	const regular = '--wpds-typography-font-weight-default'
	const small = '--wpds-typography-font-size-sm'

	expect(declarations('.alphone-tasks__open-contact')).toMatchObject({
		'font-size': `var(${small})`,
		'font-weight': `var(${regular})`,
	})
	expect(declarations('.alphone-tasks__title .alphone-tasks__open-contact')).toEqual({
		color: `var(${weak})`,
	})
})

test('darkens the contact name while it is pointed at, pressed or focused, like a quiet WordPress button', () => {
	const active = '--wpds-color-foreground-interactive-neutral-active'

	expect(declarations('.alphone-tasks__title .alphone-tasks__open-contact:is(:hover, :active, :focus)')).toEqual({
		color: `var(${active})`,
	})
})

test('puts the contact name on its own line under the title on a phone, its text flush with the title', () => {
	const padding = '--wpds-dimension-padding-sm'
	const gap = '--wpds-dimension-gap-xs'

	expect(declarations('.alphone-tasks__open-contact', '(max-width: 639px)')).toEqual({
		display: 'flex',
		width: 'fit-content',
		'max-width': `calc(100% + var(${padding}))`,
		'margin-inline-start': `calc(-1 * var(${padding}))`,
		'margin-block': `var(${gap}) 0`,
	})
})

test('keeps a long contact name on one line, cut short with an ellipsis', () => {
	expect(declarations('.alphone-tasks__contact-name')).toEqual({
		'min-width': '0',
		overflow: 'hidden',
		'text-overflow': 'ellipsis',
		'white-space': 'nowrap',
	})
})

test('keeps the person icon its full size beside a name cut short', () => {
	expect(declarations('.alphone-tasks__contact-icon')).toEqual({
		flex: 'none',
	})
})

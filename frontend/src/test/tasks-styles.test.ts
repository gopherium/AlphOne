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

test('puts the contact name on its own line under the title on a phone, its icon flush with the title', () => {
	const padding = '--wpds-dimension-padding-sm'
	const border = '--wpds-border-width-xs'
	const gap = '--wpds-dimension-gap-xs'

	expect(declarations('.alphone-tasks__open-contact', '(max-width: 639px)')).toEqual({
		display: 'flex',
		width: 'fit-content',
		'max-width': `calc(100% + var(${padding}) + var(${border}))`,
		'margin-inline-start': `calc(-1 * (var(${padding}) + var(${border})))`,
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

test('gives the time column one width on every row, the widest times of the format locale held in it unseen', () => {
	const weak = '--wpds-color-foreground-content-neutral-weak'

	expect(declarations('.alphone-tasks__time')).toEqual({
		display: 'flex',
		flex: 'none',
		'flex-direction': 'column',
		color: `var(${weak})`,
		'font-variant-numeric': 'tabular-nums',
		'white-space': 'nowrap',
	})
	expect(declarations('.alphone-tasks__time::before')).toEqual({ content: 'attr(data-morning)' })
	expect(declarations('.alphone-tasks__time::after')).toEqual({ content: 'attr(data-evening)' })
	expect(declarations('.alphone-tasks__time::before,\n.alphone-tasks__time::after')).toEqual({
		height: '0',
		overflow: 'hidden',
		visibility: 'hidden',
	})
})

test('gives the time and the title one lead group that takes the row share, the title taking the rest of it', () => {
	const phone = '(max-width: 639px)'

	expect(declarations('.alphone-tasks__lead')).toEqual({ flex: '1', 'min-width': '0' })
	expect(declarations('.alphone-tasks__contact-block .alphone-tasks__lead')).toEqual({ flex: '1 1 50%' })
	expect(declarations('.alphone-tasks__lead', phone)).toEqual({ flex: '1 1 60%' })
	expect(declarations('.alphone-tasks__title')).toEqual({ flex: '1', 'min-width': '0' })
	expect(declarations('.alphone-tasks__contact-block .alphone-tasks__title')).toEqual({})
	expect(declarations('.alphone-tasks__title', phone)).toEqual({})
})

test('keeps the person icon its full size beside a name cut short', () => {
	expect(declarations('.alphone-tasks__contact-icon')).toEqual({
		flex: 'none',
	})
})

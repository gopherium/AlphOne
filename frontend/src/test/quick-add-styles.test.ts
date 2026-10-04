// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { declarations } from './stylesheet'

/** fields selects the quick add date and priority, every row child but the title and the button. */
const fields = '.alphone-tasks__add .godmin-form__row > :not(.godmin-form__grow, button)'

/** phone is the condition of the small viewport block. */
const phone = '(max-width: 639px)'

test('lets only the quick add title grow, the date and the priority keeping their basis', () => {
	expect(declarations(fields)).toEqual({ 'flex-grow': '0' })
})

test('gives the quick add title its own line on a phone, the date and the priority sharing the next', () => {
	expect(declarations('.alphone-tasks__add .godmin-form__grow', phone)).toEqual({ 'flex-basis': '100%' })
	expect(declarations(fields, phone)).toEqual({ 'flex-grow': '1' })
})

test('keeps the media lookup inside its block', () => {
	expect(declarations('.alphone-tasks__title', phone)).toEqual({ flex: '1 1 60%' })
	expect(declarations('.alphone-tasks__title')).toEqual({ flex: '1', 'min-width': '0' })
})

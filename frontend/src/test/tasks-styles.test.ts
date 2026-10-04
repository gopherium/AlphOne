// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { declarations } from './stylesheet'

test('sets the person icon on the title line, a small gap after its last word, without growing the line', () => {
	const gap = '--wpds-dimension-gap-xs'
	const line = '--wpds-typography-line-height-sm'
	const button = '--wpds-dimension-size-sm'

	expect(declarations('.alphone-tasks__open-contact')).toEqual({
		'vertical-align': 'top',
		'margin-inline-start': `var(${gap})`,
		'margin-block': `calc((var(${line}) - var(${button})) / 2)`,
	})
})

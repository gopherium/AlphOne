// SPDX-License-Identifier: AGPL-3.0-or-later

import { ButtonLink as wordpressButtonLink } from '@wordpress/ui'
import { expect, test } from 'vitest'

import { ButtonLink } from '../index'

test('hands plugins the WordPress link drawn as a button, with its icon part', () => {
	expect(ButtonLink).toBe(wordpressButtonLink)
	expect(ButtonLink.Icon).toBe(wordpressButtonLink.Icon)
})
